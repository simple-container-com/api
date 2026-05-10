package actions

import "testing"

func TestExecutor_IsPreviewMode(t *testing.T) {
	// Env vars that should each individually flip preview mode on.
	previewEnvVars := []string{
		"PREVIEW_ONLY",
		"SC_PREVIEW",
		"SC_DRY_RUN",
		"DRY_RUN",
		"SC_DEPLOY_PREVIEW",
	}

	clearEnv := func() {
		for _, v := range previewEnvVars {
			t.Setenv(v, "")
		}
		t.Setenv("GITHUB_EVENT_NAME", "")
	}

	executor := &Executor{}

	t.Run("all unset returns false", func(t *testing.T) {
		clearEnv()
		if executor.isPreviewMode() {
			t.Fatal("isPreviewMode() with no env set should return false")
		}
	})

	for _, name := range previewEnvVars {
		t.Run(name+"=true triggers preview", func(t *testing.T) {
			clearEnv()
			t.Setenv(name, "true")
			if !executor.isPreviewMode() {
				t.Fatalf("isPreviewMode() with %s=true should return true", name)
			}
		})

		t.Run(name+" non-true value does not trigger preview", func(t *testing.T) {
			clearEnv()
			// Anything other than the literal string "true" must not trigger preview —
			// guards against e.g. "false" or "1" producing a surprising mode flip.
			t.Setenv(name, "1")
			if executor.isPreviewMode() {
				t.Fatalf("isPreviewMode() with %s=1 should return false", name)
			}
		})
	}

	t.Run("GITHUB_EVENT_NAME=pull_request triggers preview", func(t *testing.T) {
		clearEnv()
		t.Setenv("GITHUB_EVENT_NAME", "pull_request")
		if !executor.isPreviewMode() {
			t.Fatal("isPreviewMode() with GITHUB_EVENT_NAME=pull_request should return true")
		}
	})

	t.Run("GITHUB_EVENT_NAME=push does not trigger preview", func(t *testing.T) {
		clearEnv()
		t.Setenv("GITHUB_EVENT_NAME", "push")
		if executor.isPreviewMode() {
			t.Fatal("isPreviewMode() with GITHUB_EVENT_NAME=push should return false")
		}
	})
}
