// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package imageship

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"github.com/moby/moby/api/types/image"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

const (
	testRef = "ghcr.io/org/app:v1"
	testID  = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	otherID = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

// A stand-in for a saved image: `docker load` only ever sees a byte stream, so the
// only thing worth asserting about it is that exactly these bytes arrive.
var testTar = bytes.Repeat([]byte("sc-image-tar-"), 4096)

type fakeDocker struct {
	inspect    image.InspectResponse
	inspectErr error
	// byRef overrides inspect for a specific reference, for the multi-image cases.
	byRef map[string]image.InspectResponse
	// absent are references ImageInspect refuses until they have been pulled.
	absent    map[string]bool
	pullErr   error
	tar       []byte
	saveErr   error
	saveCalls int
	// saved is the ref list of each ImageSave call, in order.
	saved  [][]string
	pulled []string
}

func (f *fakeDocker) ImageInspect(_ context.Context, ref string) (image.InspectResponse, error) {
	if f.absent[ref] {
		return image.InspectResponse{}, errors.Errorf("Error: No such image: %s", ref)
	}
	if f.inspectErr != nil {
		return image.InspectResponse{}, f.inspectErr
	}
	if res, ok := f.byRef[ref]; ok {
		return res, nil
	}
	return f.inspect, nil
}

func (f *fakeDocker) ImagePull(_ context.Context, ref string) error {
	f.pulled = append(f.pulled, ref)
	if f.pullErr != nil {
		return f.pullErr
	}
	delete(f.absent, ref)
	return nil
}

func (f *fakeDocker) ImageSave(_ context.Context, refs []string) (io.ReadCloser, error) {
	f.saveCalls++
	f.saved = append(f.saved, refs)
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return io.NopCloser(bytes.NewReader(f.tar)), nil
}

type fakeRemote struct {
	platform    string
	platformErr error
	// imageIDs answers the successive `docker image inspect` calls. An empty entry
	// stands for "no such image", which is an error on the wire, not an empty id.
	// Used by the single-image tests, where call order is unambiguous.
	imageIDs []string
	// state is the alternative for the multi-image tests: the machine's image table,
	// keyed by reference, which `docker load` then updates from loads. Set it and
	// imageIDs is ignored.
	state map[string]string
	loads map[string]string

	loadErr error

	cmds     []string
	received []byte
}

func (f *fakeRemote) Describe() string { return "sc@203.0.113.10:22" }

// refOf pulls the reference back out of `docker image inspect --format '…' 'ref'`.
func refOf(cmd string) string {
	parts := strings.Split(cmd, "'")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

func (f *fakeRemote) Run(_ context.Context, cmd string, stdin io.Reader) (string, error) {
	f.cmds = append(f.cmds, cmd)
	switch {
	case strings.HasPrefix(cmd, "docker version"):
		if f.platformErr != nil {
			return "", f.platformErr
		}
		return f.platform, nil
	case strings.HasPrefix(cmd, "docker image inspect"):
		if f.state != nil {
			ref := refOf(cmd)
			if id, ok := f.state[ref]; ok && id != "" {
				return id, nil
			}
			return "", errors.Errorf("Error: No such image: %s", ref)
		}
		if len(f.imageIDs) == 0 {
			return "", errors.Errorf("Error: No such image: %s", testRef)
		}
		id := f.imageIDs[0]
		f.imageIDs = f.imageIDs[1:]
		if id == "" {
			return "", errors.Errorf("Error: No such image: %s", testRef)
		}
		return id, nil
	case strings.HasPrefix(cmd, "docker load"):
		received, err := io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
		f.received = received
		if f.loadErr != nil {
			return "", f.loadErr
		}
		for ref, id := range f.loads {
			f.state[ref] = id
		}
		return "Loaded image: " + testRef, nil
	}
	return "", errors.Errorf("unexpected command %q", cmd)
}

func linuxAmd64() *fakeDocker {
	return &fakeDocker{
		inspect: image.InspectResponse{ID: testID, Os: "linux", Architecture: "amd64"},
		tar:     testTar,
	}
}

func shipOpts(mutate func(*Options)) Options {
	o := Options{Images: []string{testRef}, Log: func(string, ...any) {}}
	mutate(&o)
	return o
}

// first is the per-image result of a single-image ship, which most of these tests do.
func first(r *Result) ImageResult {
	Expect(r.Images).To(HaveLen(1))
	return r.Images[0]
}

// The idempotency contract: the second deploy of an unchanged image must not move
// hundreds of megabytes across a link to another country. The check is by image id,
// which is the one identity `docker save`/`docker load` preserves.
func TestShipSkipsWhenTheMachineAlreadyHoldsTheImage(t *testing.T) {
	RegisterTestingT(t)

	docker := linuxAmd64()
	remote := &fakeRemote{platform: "linux/amd64", imageIDs: []string{testID}}

	res, err := Ship(context.Background(), docker, remote, shipOpts(func(*Options) {}))
	Expect(err).To(BeNil())
	Expect(res.SkippedAll()).To(BeTrue())
	Expect(first(res).Skipped).To(BeTrue())
	Expect(docker.saveCalls).To(Equal(0))
	Expect(remote.received).To(BeEmpty())
}

func TestShipForceOverridesTheSkip(t *testing.T) {
	RegisterTestingT(t)

	docker := linuxAmd64()
	remote := &fakeRemote{platform: "linux/amd64", imageIDs: []string{testID, testID}}

	res, err := Ship(context.Background(), docker, remote, shipOpts(func(o *Options) { o.Force = true }))
	Expect(err).To(BeNil())
	Expect(res.SkippedAll()).To(BeFalse())
	Expect(docker.saveCalls).To(Equal(1))
}

// What arrives at `docker load` must be byte-for-byte what `docker save` produced,
// for every compression. A truncated final frame is the failure mode here (a
// compressor that is not Closed), and it shows up as an opaque "unexpected EOF"
// from the far end rather than as anything pointing at this code.
func TestShipStreamsTheSavedTarIntoDockerLoadIntact(t *testing.T) {
	RegisterTestingT(t)

	for _, compression := range []string{CompressionZstd, CompressionGzip, CompressionNone, ""} {
		docker := linuxAmd64()
		remote := &fakeRemote{platform: "linux/amd64", imageIDs: []string{"", testID}}

		res, err := Ship(context.Background(), docker, remote,
			shipOpts(func(o *Options) { o.Compression = compression }))
		Expect(err).To(BeNil(), compression)
		Expect(res.SkippedAll()).To(BeFalse(), compression)
		Expect(first(res).ImageID).To(Equal(testID), compression)

		Expect(decompress(compression, remote.received)).To(Equal(testTar), compression)
		Expect(res.BytesRaw).To(Equal(int64(len(testTar))), compression)
		Expect(res.BytesCompressed).To(Equal(int64(len(remote.received))), compression)
		if compression != CompressionNone {
			// Not a performance assertion — it catches a stream that was passed
			// through uncompressed while reporting a compression.
			Expect(res.BytesCompressed).To(BeNumerically("<", res.BytesRaw), compression)
		}
	}
}

func TestShipRefusesAnUnknownCompression(t *testing.T) {
	RegisterTestingT(t)

	_, err := Ship(context.Background(), linuxAmd64(), &fakeRemote{platform: "linux/amd64"},
		shipOpts(func(o *Options) { o.Compression = "lz4" }))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(`unknown compression "lz4"`))
}

// `docker load` accepts a foreign-arch image without a word, and the failure then
// surfaces much later as a container exiting with "exec format error" — far from
// the deploy that caused it. So the gate is here, before any bytes move.
func TestShipRefusesAnArchMismatch(t *testing.T) {
	RegisterTestingT(t)

	docker := linuxAmd64()
	remote := &fakeRemote{platform: "linux/arm64", imageIDs: []string{"", testID}}

	_, err := Ship(context.Background(), docker, remote, shipOpts(func(*Options) {}))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("linux/amd64"))
	Expect(err.Error()).To(ContainSubstring("linux/arm64"))
	Expect(err.Error()).To(ContainSubstring("exec format error"))
	Expect(docker.saveCalls).To(Equal(0))
}

func TestShipAllowsAnArchMismatchWhenAskedTo(t *testing.T) {
	RegisterTestingT(t)

	var logged []string
	docker := linuxAmd64()
	remote := &fakeRemote{platform: "linux/arm64", imageIDs: []string{"", testID}}

	res, err := Ship(context.Background(), docker, remote, shipOpts(func(o *Options) {
		o.AllowArchMismatch = true
		o.Log = func(format string, args ...any) { logged = append(logged, format) }
	}))
	Expect(err).To(BeNil())
	Expect(res.RemotePlatform).To(Equal("linux/arm64"))
	Expect(docker.saveCalls).To(Equal(1))
	Expect(strings.Join(logged, "\n")).To(ContainSubstring("WARNING"))
}

// The variant is part of the platform. Deciding that linux/arm64/v8 is close enough
// to linux/arm64 would be exactly the silent assumption this gate exists to refuse —
// and the gate reads the IMAGE's platform, not the runner's, because a CI runner
// cross-building for the machine is the normal case rather than an error.
func TestShipTreatsTheVariantAsPartOfThePlatform(t *testing.T) {
	RegisterTestingT(t)

	docker := &fakeDocker{
		inspect: image.InspectResponse{ID: testID, Os: "linux", Architecture: "arm64", Variant: "v8"},
		tar:     testTar,
	}
	remote := &fakeRemote{platform: "linux/arm64", imageIDs: []string{"", testID}}

	_, err := Ship(context.Background(), docker, remote, shipOpts(func(*Options) {}))
	// linux/arm64/v8 against a daemon reporting linux/arm64 is still a refusal: the
	// variant is part of the platform, and guessing that v8 is compatible is exactly
	// the kind of silent assumption this gate exists to avoid.
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("linux/arm64/v8"))
}

// `docker load` exits 0 in cases where the reference does not end up pointing at
// our image, so success is verified by id rather than by exit status.
func TestShipFailsWhenTheLoadedImageIsNotTheOneSent(t *testing.T) {
	RegisterTestingT(t)

	docker := linuxAmd64()
	remote := &fakeRemote{platform: "linux/amd64", imageIDs: []string{"", otherID}}

	_, err := Ship(context.Background(), docker, remote, shipOpts(func(*Options) {}))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("resolves to 222222222222"))
	Expect(err.Error()).To(ContainSubstring("expected 111111111111"))
}

func TestShipReportsALoadFailureWithTheRemoteStderr(t *testing.T) {
	RegisterTestingT(t)

	remote := &fakeRemote{
		platform: "linux/amd64",
		imageIDs: []string{""},
		loadErr:  errors.New("no space left on device"),
	}
	_, err := Ship(context.Background(), linuxAmd64(), remote, shipOpts(func(*Options) {}))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("no space left on device"))
	Expect(err.Error()).To(ContainSubstring("`docker load` failed"))
}

// SSH answers well before cloud-init has finished installing docker, so "docker not
// found" on a machine created a minute ago means "wait", not "broken". The message
// has to say so, or the operator goes looking for a provisioning bug.
func TestShipErrorNamesCloudInitWhenDockerIsNotThereYet(t *testing.T) {
	RegisterTestingT(t)

	remote := &fakeRemote{platformErr: errors.New("bash: docker: command not found")}
	_, err := Ship(context.Background(), linuxAmd64(), remote, shipOpts(func(*Options) {}))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("cloud-init"))
}

func TestShipErrorNamesTheLocalDockerWhenTheImageIsNotBuilt(t *testing.T) {
	RegisterTestingT(t)

	docker := linuxAmd64()
	docker.inspectErr = errors.New("No such image")
	_, err := Ship(context.Background(), docker, &fakeRemote{platform: "linux/amd64"}, shipOpts(func(*Options) {}))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("not in the local docker"))
	// And it names the way out, because the common case for an image that is not
	// there is one SC did not build: a third-party compose image, or the ingress.
	Expect(err.Error()).To(ContainSubstring("--pull"))
	Expect(docker.pulled).To(BeEmpty())
}

// --pull is what makes shipping usable for images SC does not build. It fetches only
// what is ABSENT: an image the runner already holds is shipped as it stands, so a
// deploy does not re-pull a pinned third-party tag on every run.
func TestShipPullsOnlyTheImagesTheRunnerDoesNotHave(t *testing.T) {
	RegisterTestingT(t)

	const mongo = "mongo:7"
	docker := linuxAmd64()
	docker.byRef = map[string]image.InspectResponse{
		testRef: {ID: testID, Os: "linux", Architecture: "amd64"},
		mongo:   {ID: otherID, Os: "linux", Architecture: "amd64"},
	}
	docker.absent = map[string]bool{mongo: true}
	remote := &fakeRemote{
		platform: "linux/amd64",
		state:    map[string]string{},
		loads:    map[string]string{testRef: testID, mongo: otherID},
	}

	res, err := Ship(context.Background(), docker, remote, shipOpts(func(o *Options) {
		o.Images = []string{testRef, mongo}
		o.Pull = true
	}))
	Expect(err).To(BeNil())
	Expect(docker.pulled).To(Equal([]string{mongo}))
	Expect(res.Images).To(HaveLen(2))
	Expect(res.Images[0].Pulled).To(BeFalse())
	Expect(res.Images[1].Pulled).To(BeTrue())
	// One stream for both, which is the point: `docker save` deduplicates the layers
	// they share, and a second invocation would mean a second SSH connection and a
	// second compressor.
	Expect(docker.saved).To(Equal([][]string{{testRef, mongo}}))
}

// A pull that fails is the deploy's failure, and the message has to carry the
// registry's own reason: a 401 on a private image and "no route to host" from inside a
// closed network are the same symptom here and need opposite fixes.
func TestShipReportsWhyAPullFailed(t *testing.T) {
	RegisterTestingT(t)

	docker := linuxAmd64()
	docker.absent = map[string]bool{testRef: true}
	docker.pullErr = errors.New("unauthorized: authentication required")

	_, err := Ship(context.Background(), docker, &fakeRemote{platform: "linux/amd64"},
		shipOpts(func(o *Options) { o.Pull = true }))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("authentication required"))
	Expect(err.Error()).To(ContainSubstring("could not be pulled here"))
}

// Per-image idempotency, not all-or-nothing: the stack's own image changes on every
// deploy while the mongo beside it does not, so a deploy must move the one and not
// the other.
func TestShipSkipsOnlyTheImagesTheMachineAlreadyHolds(t *testing.T) {
	RegisterTestingT(t)

	const mongo = "mongo:7"
	docker := linuxAmd64()
	docker.byRef = map[string]image.InspectResponse{
		testRef: {ID: testID, Os: "linux", Architecture: "amd64"},
		mongo:   {ID: otherID, Os: "linux", Architecture: "amd64"},
	}
	remote := &fakeRemote{
		platform: "linux/amd64",
		state:    map[string]string{mongo: otherID},
		loads:    map[string]string{testRef: testID},
	}

	res, err := Ship(context.Background(), docker, remote, shipOpts(func(o *Options) {
		o.Images = []string{testRef, mongo}
	}))
	Expect(err).To(BeNil())
	Expect(res.SkippedAll()).To(BeFalse())
	Expect(res.Shipped()).To(Equal([]string{testRef}))
	Expect(docker.saved).To(Equal([][]string{{testRef}}))
}

// Two services of one compose project running the same third-party image is ordinary,
// and the callers that assemble the list walk services. `docker save` given the same
// reference twice is not something to find out about on the wire.
func TestShipDeduplicatesTheImageList(t *testing.T) {
	RegisterTestingT(t)

	docker := linuxAmd64()
	remote := &fakeRemote{
		platform: "linux/amd64",
		state:    map[string]string{},
		loads:    map[string]string{testRef: testID},
	}

	res, err := Ship(context.Background(), docker, remote, shipOpts(func(o *Options) {
		o.Images = []string{testRef, testRef}
	}))
	Expect(err).To(BeNil())
	Expect(res.Images).To(HaveLen(1))
	Expect(docker.saved).To(Equal([][]string{{testRef}}))
}

func TestShipRefusesAnEmptyImageList(t *testing.T) {
	RegisterTestingT(t)

	_, err := Ship(context.Background(), linuxAmd64(), &fakeRemote{platform: "linux/amd64"},
		shipOpts(func(o *Options) { o.Images = nil }))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("no image to ship"))
}

// The gates run over every image before the first byte moves. A mismatched arch in
// position two must not be discovered with position one already on the machine —
// half a stack's images loaded is a worse state than none of them.
func TestShipGatesEveryImageBeforeTransferringAny(t *testing.T) {
	RegisterTestingT(t)

	const arm = "ghcr.io/org/other:v1"
	docker := linuxAmd64()
	docker.byRef = map[string]image.InspectResponse{
		testRef: {ID: testID, Os: "linux", Architecture: "amd64"},
		arm:     {ID: otherID, Os: "linux", Architecture: "arm64"},
	}
	remote := &fakeRemote{platform: "linux/amd64", state: map[string]string{}}

	_, err := Ship(context.Background(), docker, remote, shipOpts(func(o *Options) {
		o.Images = []string{testRef, arm}
	}))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(arm))
	Expect(docker.saveCalls).To(Equal(0))
}

// An image reference reaches a remote shell, so it is quoted — and, more importantly,
// it was validated against security.ValidateImageRef by the caller, whose pattern
// admits no shell metacharacter at all.
func TestShipQuotesTheReferenceItPassesToTheRemoteShell(t *testing.T) {
	RegisterTestingT(t)

	remote := &fakeRemote{platform: "linux/amd64", imageIDs: []string{"", testID}}
	_, err := Ship(context.Background(), linuxAmd64(), remote, shipOpts(func(*Options) {}))
	Expect(err).To(BeNil())
	Expect(remote.cmds).To(ContainElement(`docker image inspect --format '{{.Id}}' 'ghcr.io/org/app:v1'`))
	Expect(remote.cmds).To(ContainElement(`docker version --format '{{.Server.Os}}/{{.Server.Arch}}'`))
	Expect(remote.cmds).To(ContainElement("docker load -q"))
}

func decompress(compression string, in []byte) []byte {
	switch compression {
	case CompressionNone:
		return in
	case CompressionGzip:
		r, err := gzip.NewReader(bytes.NewReader(in))
		Expect(err).To(BeNil())
		out, err := io.ReadAll(r)
		Expect(err).To(BeNil())
		return out
	default:
		r, err := zstd.NewReader(bytes.NewReader(in))
		Expect(err).To(BeNil())
		defer r.Close()
		out, err := io.ReadAll(r)
		Expect(err).To(BeNil())
		return out
	}
}
