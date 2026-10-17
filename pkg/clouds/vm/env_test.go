// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

func envInput(cfg *api.StackConfigCompose) *VmComposeInput {
	return &VmComposeInput{Deployment: k8s.DeploymentConfig{StackConfig: cfg}}
}

// Precedence has to match every other target, or moving a stack between parents changes
// which value wins without the client.yaml changing.
func TestRenderEnvFilePrecedenceAndOrder(t *testing.T) {
	RegisterTestingT(t)

	out, err := RenderEnvFile(envInput(&api.StackConfigCompose{
		Env: map[string]string{
			"NODE_ENV":  "production",
			"MONGO_URI": "from-env",
			"API_KEY":   "from-env",
		},
		Secrets: map[string]string{"API_KEY": "from-secrets"},
	}), map[string]string{"MONGO_URI": "from-resource-context"})
	Expect(err).ToNot(HaveOccurred())

	// Sorted, so the file is byte-stable across deploys and a diff is readable.
	Expect(out).To(Equal(strings.Join([]string{
		"API_KEY=from-secrets",
		"MONGO_URI=from-resource-context",
		"NODE_ENV=production",
		"",
	}, "\n")))
}

func TestRenderEnvFileEscapesNewlinesWithoutQuoting(t *testing.T) {
	RegisterTestingT(t)

	out, err := RenderEnvFile(envInput(&api.StackConfigCompose{
		Secrets: map[string]string{
			"PRIVATE_KEY": "line1\nline2\r\nline3\rline4",
			"WITH_SPACES": "a b c",
			"WITH_QUOTES": `say "hi"`,
			"WITH_HASH":   "value # not-a-comment",
		},
	}), nil)
	Expect(err).ToNot(HaveOccurred())

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	// One line per variable: a literal newline would make docker read the rest of the
	// value as another variable.
	Expect(lines).To(HaveLen(4))
	Expect(lines).To(ContainElement(`PRIVATE_KEY=line1\nline2\nline3\nline4`))
	// Docker's env-file parser is not a shell: it takes the rest of the line verbatim,
	// so quoting would make the quotes part of the value.
	Expect(lines).To(ContainElement("WITH_SPACES=a b c"))
	Expect(lines).To(ContainElement(`WITH_QUOTES=say "hi"`))
	Expect(lines).To(ContainElement("WITH_HASH=value # not-a-comment"))
}

func TestRenderEnvFileRefusesUnusableKeys(t *testing.T) {
	RegisterTestingT(t)

	for _, key := range []string{"with space", "with-dash", "1LEADING_DIGIT", "WITH=EQUALS", ""} {
		// Docker would accept the line and produce a variable the application can
		// never read, which diagnoses as the application being broken.
		_, err := RenderEnvFile(envInput(&api.StackConfigCompose{
			Env: map[string]string{key: "x"},
		}), nil)
		Expect(err).To(MatchError(ContainSubstring("cannot be used")), "key %q should be refused", key)
	}

	_, err := RenderEnvFile(nil, nil)
	Expect(err).To(MatchError(ContainSubstring("input is nil")))
}

func TestRenderEnvFileEmpty(t *testing.T) {
	RegisterTestingT(t)

	out, err := RenderEnvFile(envInput(&api.StackConfigCompose{}), nil)
	Expect(err).ToNot(HaveOccurred())
	// Empty rather than a header: an `env_file` is parsed as data, and keeping it to
	// exactly the data makes a corrupted file obvious.
	Expect(out).To(BeEmpty())
}

func TestTextVolumeFiles(t *testing.T) {
	RegisterTestingT(t)

	input := envInput(&api.StackConfigCompose{})
	input.Deployment.TextVolumes = []k8s.SimpleTextVolume{
		{TextVolume: api.TextVolume{Name: "nginx.conf", MountPath: "/etc/nginx/nginx.conf", Content: "server {}"}},
	}

	files := TextVolumeFiles(input, "refapp", "staging")
	Expect(files).To(HaveLen(1))
	Expect(files[0].Path).To(Equal("/etc/sc/refapp/staging/volumes/nginx.conf"))
	Expect(files[0].MountPath).To(Equal("/etc/nginx/nginx.conf"))
	Expect(files[0].Content).To(Equal("server {}"))
	// 0644, not 0600: the file is bind-mounted and read by the CONTAINER's user, which
	// SC does not know. A 0600 file owned by the ssh user is unreadable to a container
	// running as anyone else, and it fails as the application not finding its config.
	Expect(files[0].Mode).To(Equal(TextVolumeMode))

	Expect(TextVolumeFiles(nil, "refapp", "staging")).To(BeNil())
}
