package docker

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
)

func TestStageSecurityReportScript_RoundTrip(t *testing.T) {
	RegisterTestingT(t)

	script := "echo hello\nprintf 'world\\n'\n"
	path, err := stageSecurityReportScript("security-report-baas", script)
	defer os.Remove(path)

	Expect(err).NotTo(HaveOccurred())
	Expect(path).NotTo(BeEmpty())
	Expect(path).To(HavePrefix(os.TempDir()))
	Expect(path).To(HaveSuffix(".sh"))

	got, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(got)).To(Equal(script))
}

// stageSecurityReportScript must produce the same path on identical inputs so
// Pulumi sees no drift between runs of an otherwise-unchanged resource.
func TestStageSecurityReportScript_DeterministicPath(t *testing.T) {
	RegisterTestingT(t)

	script := "REPORT=\"\"\nprintf '%b' \"$REPORT\"\n"
	p1, err := stageSecurityReportScript("security-report-api", script)
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(p1)

	p2, err := stageSecurityReportScript("security-report-api", script)
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(p2)

	Expect(p1).To(Equal(p2))
}

// Same resource name + different script content => different paths.
func TestStageSecurityReportScript_DifferentScriptDifferentPath(t *testing.T) {
	RegisterTestingT(t)

	p1, err := stageSecurityReportScript("security-report-api", "echo one\n")
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(p1)

	p2, err := stageSecurityReportScript("security-report-api", "echo two\n")
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(p2)

	Expect(p1).NotTo(Equal(p2))
}

// Resource names may contain `:` or `/` (registry URLs, etc.). The tempfile
// path must be filesystem-safe.
func TestStageSecurityReportScript_SanitizesResourceName(t *testing.T) {
	RegisterTestingT(t)

	path, err := stageSecurityReportScript(
		"security-report-471112843480.dkr.ecr.us-west-2.amazonaws.com/baas-ecr:tag",
		"echo ok\n",
	)
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(path)

	// No raw `:` or `/` should leak into the basename.
	base := path[strings.LastIndex(path, "/")+1:]
	Expect(base).NotTo(ContainSubstring(":"))
}

// Long resource names (registry hostname + slash-separated path + tag
// produced by SC's naming convention) can push the unsanitised resource
// name well past NAME_MAX (255 bytes on Linux). The helper must cap the
// resource-name component so the final basename stays under the limit —
// otherwise ENAMETOOLONG would trip the fallback path and reintroduce
// the ARG_MAX failure for the large reports this helper exists to fix.
func TestStageSecurityReportScript_CapsLongResourceName(t *testing.T) {
	RegisterTestingT(t)

	long := strings.Repeat("very-long-resource-name-segment.", 20) // ~640 chars
	path, err := stageSecurityReportScript(long, "echo ok\n")
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(path)

	base := filepath.Base(path)
	Expect(len(base)).To(BeNumerically("<", 255))
}

// `os.Rename` is atomic on the same filesystem. Concurrent writers
// targeting the same final path (same resource name + same script
// content => same path) must not produce a partially-written file
// observable by a reader. We can't directly assert atomicity, but we
// can run many writers in parallel and assert every observed final
// file is complete + correct.
func TestStageSecurityReportScript_ConcurrentWriters(t *testing.T) {
	RegisterTestingT(t)

	const script = "REPORT=\"value\"\nprintf '%s\\n' \"$REPORT\"\n"
	const n = 24

	var wg sync.WaitGroup
	wg.Add(n)
	paths := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			paths[i], errs[i] = stageSecurityReportScript("security-report-concurrent", script)
		}(i)
	}
	wg.Wait()

	for i, p := range paths {
		Expect(errs[i]).NotTo(HaveOccurred())
		Expect(p).NotTo(BeEmpty())
		got, err := os.ReadFile(p)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(got)).To(Equal(script), "writer %d observed truncated/interleaved content", i)
	}
	// All paths identical (deterministic).
	for i := 1; i < n; i++ {
		Expect(paths[i]).To(Equal(paths[0]))
	}
	defer os.Remove(paths[0])
}

// Reality check: the original ARG_MAX failure was a ~150 KB inlined script.
// A staged invocation `sh <path>` is well under the kernel limit
// (typically 128 KB on Linux). This test asserts the contract — that the
// helper produces a short path that the caller can compose into a short
// Create command, regardless of the script body size.
func TestStageSecurityReportScript_HandlesLargeScript(t *testing.T) {
	RegisterTestingT(t)

	// Build a script body that exceeds typical Linux ARG_MAX (128 KB) by
	// roughly 2× — the size class the original failure landed at on a
	// chrome-base-derived image (5,025 merged CVEs producing ~150 KB).
	const targetBytes = 256 * 1024
	var sb strings.Builder
	for sb.Len() < targetBytes {
		sb.WriteString("REPORT=\"${REPORT}| HIGH | CVE-XXXX-YYYY | pkg | x.y | - |\\n\"\n")
	}
	largeScript := sb.String()
	Expect(len(largeScript)).To(BeNumerically(">", 256*1024))

	path, err := stageSecurityReportScript("security-report-large", largeScript)
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(path)

	// Path itself stays small.
	Expect(len(path)).To(BeNumerically("<", 256))

	got, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(got)).To(Equal(len(largeScript)))
}
