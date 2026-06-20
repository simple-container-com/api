package github

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// fullEnhancedConfig returns a representative, fully-populated enhanced config
// exercising auto-deploy, protected environments, preview env, custom actions
// and validation settings, so template rendering paths are well covered.
func fullEnhancedConfig() *EnhancedActionsCiCdConfig {
	return &EnhancedActionsCiCdConfig{
		AuthToken: "ghp_token",
		Organization: OrganizationConfig{
			Name:          "acme",
			DefaultRunner: "ubuntu-latest",
			DefaultBranch: "main",
		},
		WorkflowGeneration: WorkflowGenerationConfig{
			Enabled:   true,
			Templates: []string{"deploy", "destroy", "destroy-parent", "provision", "pr-preview"},
			SCVersion: "latest",
		},
		Environments: map[string]EnvironmentConfig{
			"staging": {
				Type:       "staging",
				Runner:     "ubuntu-latest",
				AutoDeploy: true,
				Secrets:    []string{"STAGING_TOKEN"},
			},
			"production": {
				Type:       "production",
				Runner:     "ubuntu-latest",
				Protection: true,
				Reviewers:  []string{"alice"},
				Secrets:    []string{"PROD_TOKEN"},
			},
			"pr": {
				Type:   "preview",
				Runner: "ubuntu-latest",
				PRPreview: PRPreviewConfig{
					Enabled:      true,
					DomainBase:   "preview.acme.io",
					LabelTrigger: "deploy-it",
				},
				ValidationCmd: "make smoke",
			},
		},
		Execution: ExecutionConfig{
			DefaultTimeout: "45m",
			Concurrency:    ConcurrencyConfig{CancelInProgress: true},
		},
		Validation: ValidationConfig{
			TestSuites:   []string{"unit", "e2e"},
			HealthChecks: map[string]string{"/health": "liveness"},
		},
	}
}

// deterministicConfig has a single non-preview environment so that
// `envNamesExcluding` (which ranges a map in unsorted order) produces stable
// output across renders — required for round-trip equality assertions.
func deterministicConfig() *EnhancedActionsCiCdConfig {
	return &EnhancedActionsCiCdConfig{
		AuthToken: "t",
		Organization: OrganizationConfig{
			Name:          "acme",
			DefaultRunner: "ubuntu-latest",
			DefaultBranch: "main",
		},
		WorkflowGeneration: WorkflowGenerationConfig{
			Enabled:   true,
			Templates: []string{"deploy", "destroy"},
			SCVersion: "latest",
		},
		Environments: map[string]EnvironmentConfig{
			"staging": {Type: "staging", Runner: "ubuntu-latest", AutoDeploy: true},
		},
		Execution: ExecutionConfig{DefaultTimeout: "30m"},
	}
}

func TestNewWorkflowGenerator(t *testing.T) {
	RegisterTestingT(t)

	cfg := fullEnhancedConfig()
	wg := NewWorkflowGenerator(cfg, "mystack", "out/", true)

	Expect(wg).ToNot(BeNil())
	Expect(wg.stackName).To(Equal("mystack"))
	Expect(wg.outputPath).To(Equal("out/"))
	Expect(wg.skipRefresh).To(BeTrue())
	Expect(wg.templates).ToNot(BeNil())
	Expect(wg.config).To(BeIdenticalTo(cfg))
}

func TestWorkflowGenerator_LoadTemplates(t *testing.T) {
	RegisterTestingT(t)

	wg := NewWorkflowGenerator(fullEnhancedConfig(), "s", "", false)
	Expect(wg.LoadTemplates()).To(Succeed())

	for _, name := range GetWorkflowTemplateNames() {
		Expect(wg.templates).To(HaveKey(name))
	}
}

func TestWorkflowGenerator_GenerateWorkflows_WritesFiles(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	cfg := fullEnhancedConfig()
	wg := NewWorkflowGenerator(cfg, "web", dir, false)

	Expect(wg.GenerateWorkflows()).To(Succeed())

	for _, tmpl := range cfg.WorkflowGeneration.Templates {
		path := filepath.Join(dir, tmpl+"-web.yml")
		content, err := os.ReadFile(path)
		Expect(err).ToNot(HaveOccurred(), "expected %s to be written", path)
		Expect(string(content)).ToNot(BeEmpty())
	}
}

func TestWorkflowGenerator_GenerateWorkflows_UnknownTemplate(t *testing.T) {
	RegisterTestingT(t)

	cfg := fullEnhancedConfig()
	cfg.WorkflowGeneration.Templates = []string{"does-not-exist"}
	wg := NewWorkflowGenerator(cfg, "web", t.TempDir(), false)

	err := wg.GenerateWorkflows()
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("does-not-exist"))
}

func TestWorkflowGenerator_RenderedContent(t *testing.T) {
	RegisterTestingT(t)

	wg := NewWorkflowGenerator(fullEnhancedConfig(), "payments", "", false)
	Expect(wg.LoadTemplates()).To(Succeed())
	data := wg.prepareTemplateData()

	cases := []struct {
		template string
		contains []string
	}{
		{
			template: "deploy",
			contains: []string{
				"name: Deploy acme payments",
				`STACK_NAME: "payments"`,
				"cancel-in-progress: true",  // from Execution.Concurrency
				"timeout-minutes: 45",       // 45m -> 45
				"environment: ${{ github.event.inputs.environment", // protected env present
			},
		},
		{
			template: "destroy",
			contains: []string{
				"name: Destroy acme payments",
				"Type DESTROY to confirm",
				"validate-destroy",
			},
		},
		{
			template: "destroy-parent",
			contains: []string{
				"name: Destroy acme Infrastructure",
				"DESTROY-INFRASTRUCTURE",
				"environment: infrastructure",
			},
		},
		{
			template: "provision",
			contains: []string{
				"name: Provision acme Infrastructure",
				"branches: [main]",
				"unit test suite", // from Validation.TestSuites
			},
		},
		{
			template: "pr-preview",
			contains: []string{
				"name: PR Preview - acme payments",
				"deploy-it",            // LabelTrigger from preview env
				"preview.acme.io",      // DomainBase from preview env
				"make smoke",           // ValidationCmd indented in
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.template, func(t *testing.T) {
			RegisterTestingT(t)
			content, err := wg.generateWorkflowContent(tc.template, data)
			Expect(err).ToNot(HaveOccurred())
			for _, sub := range tc.contains {
				Expect(content).To(ContainSubstring(sub), "template %q should contain %q", tc.template, sub)
			}
		})
	}
}

func TestWorkflowGenerator_generateWorkflowContent_Unknown(t *testing.T) {
	RegisterTestingT(t)

	wg := NewWorkflowGenerator(fullEnhancedConfig(), "s", "", false)
	Expect(wg.LoadTemplates()).To(Succeed())

	_, err := wg.generateWorkflowContent("nope", wg.prepareTemplateData())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("nope"))
}

func TestWorkflowGenerator_RenderedContent_Defaults(t *testing.T) {
	RegisterTestingT(t)

	// Minimal config: no runner, no timeout, no environments -> defaults kick in.
	cfg := &EnhancedActionsCiCdConfig{
		Organization:       OrganizationConfig{Name: "min"},
		WorkflowGeneration: WorkflowGenerationConfig{Templates: []string{"deploy"}},
	}
	wg := NewWorkflowGenerator(cfg, "s", "", true) // skipRefresh true
	Expect(wg.LoadTemplates()).To(Succeed())
	content, err := wg.generateWorkflowContent("deploy", wg.prepareTemplateData())
	Expect(err).ToNot(HaveOccurred())

	Expect(content).To(ContainSubstring("runs-on: ubuntu-latest")) // default runner
	Expect(content).To(ContainSubstring("timeout-minutes: 30"))    // default timeout
	Expect(content).To(ContainSubstring("skip-refresh: \"true\"")) // skipRefresh propagated
	// No protected env -> no top-level job environment line for deploy
	Expect(content).ToNot(ContainSubstring("environment: ${{ github.event.inputs.environment"))
}

func TestWorkflowGenerator_prepareTemplateData(t *testing.T) {
	RegisterTestingT(t)

	t.Run("defaults applied when empty", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &EnhancedActionsCiCdConfig{Organization: OrganizationConfig{Name: "o"}}
		wg := NewWorkflowGenerator(cfg, "stk", "", false)
		data := wg.prepareTemplateData()

		Expect(data.StackName).To(Equal("stk"))
		Expect(data.SCVersion).To(Equal("latest"))
		Expect(data.Execution.Concurrency.Group).To(ContainSubstring("deploy-stk"))
		Expect(data.CustomActions).To(HaveKey("deploy"))
		Expect(data.CustomActions["deploy"]).To(ContainSubstring("@main"))
	})

	t.Run("scversion drives action tags", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &EnhancedActionsCiCdConfig{
			Organization:       OrganizationConfig{Name: "o"},
			WorkflowGeneration: WorkflowGenerationConfig{SCVersion: "2026.6.0"},
		}
		wg := NewWorkflowGenerator(cfg, "stk", "", false)
		data := wg.prepareTemplateData()
		Expect(data.SCVersion).To(Equal("2026.6.0"))
		Expect(data.CustomActions["deploy"]).To(ContainSubstring("@2026.6.0"))
	})

	t.Run("explicit concurrency group preserved", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &EnhancedActionsCiCdConfig{
			Organization: OrganizationConfig{Name: "o"},
			Execution:    ExecutionConfig{Concurrency: ConcurrencyConfig{Group: "custom-group"}},
		}
		wg := NewWorkflowGenerator(cfg, "stk", "", false)
		Expect(wg.prepareTemplateData().Execution.Concurrency.Group).To(Equal("custom-group"))
	})
}

func TestWorkflowGenerator_getDefaultEnvironment(t *testing.T) {
	cases := []struct {
		name string
		envs map[string]EnvironmentConfig
		want string
	}{
		{
			name: "auto-deploy wins",
			envs: map[string]EnvironmentConfig{"x": {Type: "production", AutoDeploy: true}},
			want: "x",
		},
		{
			name: "staging by type",
			envs: map[string]EnvironmentConfig{"s": {Type: "staging"}},
			want: "s",
		},
		{
			name: "staging by name",
			envs: map[string]EnvironmentConfig{"staging": {Type: "other"}},
			want: "staging",
		},
		{
			name: "production by type",
			envs: map[string]EnvironmentConfig{"p": {Type: "production"}},
			want: "p",
		},
		{
			name: "empty -> staging fallback",
			envs: map[string]EnvironmentConfig{},
			want: "staging",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			wg := NewWorkflowGenerator(&EnhancedActionsCiCdConfig{Environments: tc.envs}, "s", "", false)
			Expect(wg.getDefaultEnvironment()).To(Equal(tc.want))
		})
	}
}

func TestWorkflowGenerator_ValidateWorkflows(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	cfg := deterministicConfig()
	wg := NewWorkflowGenerator(cfg, "web", dir, false)

	t.Run("all missing", func(t *testing.T) {
		RegisterTestingT(t)
		res, err := wg.ValidateWorkflows()
		Expect(err).ToNot(HaveOccurred())
		Expect(res.IsValid).To(BeFalse())
		Expect(res.MissingFiles).To(HaveLen(2))
		Expect(res.TotalIssues()).To(Equal(2))
	})

	t.Run("valid after generation", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(wg.GenerateWorkflows()).To(Succeed())
		res, err := wg.ValidateWorkflows()
		Expect(err).ToNot(HaveOccurred())
		Expect(res.IsValid).To(BeTrue())
		Expect(res.ValidFiles).To(HaveLen(2))
		Expect(res.TotalIssues()).To(Equal(0))
	})

	t.Run("outdated when content drifts", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(os.WriteFile(filepath.Join(dir, "deploy-web.yml"), []byte("drifted"), 0o644)).To(Succeed())
		res, err := wg.ValidateWorkflows()
		Expect(err).ToNot(HaveOccurred())
		Expect(res.IsValid).To(BeFalse())
		Expect(res.OutdatedFiles).To(ContainElement("deploy-web.yml"))
	})
}

func TestWorkflowGenerator_GetSyncPlan_And_Sync(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	cfg := deterministicConfig()
	wg := NewWorkflowGenerator(cfg, "web", dir, false)

	plan, err := wg.GetSyncPlan()
	Expect(err).ToNot(HaveOccurred())
	Expect(plan.IsUpToDate()).To(BeFalse())
	Expect(plan.FilesToCreate).To(HaveLen(2))

	Expect(wg.SyncWorkflows(plan)).To(Succeed())

	// After creation, plan should be up to date.
	plan2, err := wg.GetSyncPlan()
	Expect(err).ToNot(HaveOccurred())
	Expect(plan2.IsUpToDate()).To(BeTrue())

	// Drift one file -> update detected.
	Expect(os.WriteFile(filepath.Join(dir, "deploy-web.yml"), []byte("x"), 0o644)).To(Succeed())
	plan3, err := wg.GetSyncPlan()
	Expect(err).ToNot(HaveOccurred())
	Expect(plan3.FilesToUpdate).To(HaveLen(1))
	Expect(plan3.FilesToUpdate[0].File).To(Equal("deploy-web.yml"))
	Expect(wg.SyncWorkflows(plan3)).To(Succeed())
}

func TestWorkflowGenerator_SyncWorkflows_RemovesFiles(t *testing.T) {
	RegisterTestingT(t)

	dir := t.TempDir()
	wg := NewWorkflowGenerator(fullEnhancedConfig(), "web", dir, false)
	stale := filepath.Join(dir, "stale-web.yml")
	Expect(os.WriteFile(stale, []byte("old"), 0o644)).To(Succeed())

	plan := &SyncPlan{FilesToRemove: []string{"stale-web.yml"}}
	Expect(wg.SyncWorkflows(plan)).To(Succeed())
	_, err := os.Stat(stale)
	Expect(os.IsNotExist(err)).To(BeTrue())
}

func TestWorkflowGenerator_PreviewWorkflow_Method(t *testing.T) {
	RegisterTestingT(t)

	cfg := fullEnhancedConfig()
	cfg.WorkflowGeneration.Templates = []string{"deploy", "provision"}
	wg := NewWorkflowGenerator(cfg, "api", "", false)

	preview, err := wg.PreviewWorkflow()
	Expect(err).ToNot(HaveOccurred())
	Expect(preview.StackName).To(Equal("api"))
	Expect(preview.Workflows).To(HaveLen(2))
	Expect(preview.Workflows[0].Content).ToNot(BeEmpty())
	Expect(preview.Workflows[0].FileName).To(HaveSuffix("-api.yml"))
}

func TestValidationResults_TotalIssues(t *testing.T) {
	RegisterTestingT(t)

	vr := &ValidationResults{
		MissingFiles:  []string{"a"},
		OutdatedFiles: []string{"b", "c"},
		InvalidFiles:  map[string][]string{"d": {"err"}},
	}
	Expect(vr.TotalIssues()).To(Equal(4))
}

func TestSyncPlan_IsUpToDate(t *testing.T) {
	RegisterTestingT(t)

	Expect((&SyncPlan{}).IsUpToDate()).To(BeTrue())
	Expect((&SyncPlan{FilesToCreate: []string{"a"}}).IsUpToDate()).To(BeFalse())
	Expect((&SyncPlan{FilesToUpdate: []FileUpdate{{File: "a"}}}).IsUpToDate()).To(BeFalse())
	Expect((&SyncPlan{FilesToRemove: []string{"a"}}).IsUpToDate()).To(BeFalse())
}

func TestTemplateFuncs(t *testing.T) {
	RegisterTestingT(t)
	fns := templateFuncs()

	t.Run("title", func(t *testing.T) {
		RegisterTestingT(t)
		title := fns["title"].(func(string) string)
		Expect(title("hELLO")).To(Equal("Hello"))
		Expect(title("")).To(Equal(""))
	})

	t.Run("quote", func(t *testing.T) {
		RegisterTestingT(t)
		quote := fns["quote"].(func(string) string)
		Expect(quote("x")).To(Equal(`"x"`))
	})

	t.Run("yamlList", func(t *testing.T) {
		RegisterTestingT(t)
		yamlList := fns["yamlList"].(func([]string) string)
		Expect(yamlList(nil)).To(Equal("[]"))
		Expect(yamlList([]string{"a", "b"})).To(Equal(`["a", "b"]`))
	})

	t.Run("envNamesExcluding", func(t *testing.T) {
		RegisterTestingT(t)
		fn := fns["envNamesExcluding"].(func(map[string]EnvironmentConfig, string) string)
		out := fn(map[string]EnvironmentConfig{"a": {Type: "staging"}, "b": {Type: "preview"}}, "preview")
		Expect(out).To(Equal("a"))
	})

	t.Run("timeoutMinutes", func(t *testing.T) {
		RegisterTestingT(t)
		fn := fns["timeoutMinutes"].(func(string) string)
		Expect(fn("45m")).To(Equal("45"))
		Expect(fn("60")).To(Equal("60"))
		Expect(fn("")).To(Equal("30"))
		Expect(fn("mm")).To(Equal("30"))
		// Documents a known quirk: all "m" are stripped before the "minutes"
		// replacement runs, so "minutes" never matches as a whole word.
		Expect(fn("30 minutes")).To(Equal("30 inutes"))
	})

	t.Run("defaultAction", func(t *testing.T) {
		RegisterTestingT(t)
		fn := fns["defaultAction"].(func(string, string) string)
		Expect(fn("deploy", "")).To(HaveSuffix("/deploy@main"))
		Expect(fn("deploy", "latest")).To(HaveSuffix("/deploy@main"))
		Expect(fn("deploy", "2026.6.0")).To(HaveSuffix("/deploy@2026.6.0"))
	})

	t.Run("indent", func(t *testing.T) {
		RegisterTestingT(t)
		fn := fns["indent"].(func(int, string) string)
		Expect(fn(2, "a\n\nb")).To(Equal("  a\n\n  b"))
	})

	t.Run("secretRef and envVarRef", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(fns["secretRef"].(func(string) string)("TOK")).To(Equal("${{ secrets.TOK }}"))
		Expect(fns["envVarRef"].(func(string) string)("e")).To(Equal("${{ github.event.inputs.e }}"))
	})

	t.Run("replace and string helpers", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(fns["replace"].(func(string, string, string) string)("a-b", "-", "_")).To(Equal("a_b"))
		Expect(fns["lower"].(func(string) string)("AB")).To(Equal("ab"))
		Expect(fns["upper"].(func(string) string)("ab")).To(Equal("AB"))
		Expect(fns["join"].(func([]string, string) string)([]string{"a", "b"}, ",")).To(Equal("a,b"))
		Expect(fns["contains"].(func(string, string) bool)("abc", "b")).To(BeTrue())
		Expect(fns["hasPrefix"].(func(string, string) bool)("abc", "ab")).To(BeTrue())
	})
}

func TestGetWorkflowTemplateNames(t *testing.T) {
	RegisterTestingT(t)
	names := GetWorkflowTemplateNames()
	Expect(names).To(ConsistOf("deploy", "destroy", "destroy-parent", "provision", "pr-preview"))
}

func TestConvertToEnhancedConfig(t *testing.T) {
	RegisterTestingT(t)
	// Current implementation is a stub returning an empty config; assert that
	// contract so a future real implementation forces this test to be updated.
	out, err := ConvertToEnhancedConfig(&api.Config{})
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(BeNil())
	Expect(out.AuthToken).To(Equal(""))
}

func TestValidateConfiguration(t *testing.T) {
	RegisterTestingT(t)
	// Stub ConvertToEnhancedConfig yields empty config; SetDefaults does not set
	// AuthToken, so validation fails on the required auth-token.
	err := ValidateConfiguration(&api.Config{})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("auth-token is required"))
}

func serverDescGithub() *api.ServerDescriptor {
	return &api.ServerDescriptor{
		CiCd: api.CiCdDescriptor{Type: CiCdTypeGithubActions, Config: api.Config{}},
	}
}

func TestPreviewWorkflow_PackageLevel(t *testing.T) {
	RegisterTestingT(t)

	t.Run("unsupported type", func(t *testing.T) {
		RegisterTestingT(t)
		sd := &api.ServerDescriptor{CiCd: api.CiCdDescriptor{Type: "gitlab"}}
		_, err := PreviewWorkflow(sd, "s", "deploy")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported CI/CD type"))
	})

	t.Run("renders supported template", func(t *testing.T) {
		RegisterTestingT(t)
		out, err := PreviewWorkflow(serverDescGithub(), "stk", "deploy")
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("STACK_NAME"))
	})

	t.Run("unknown template name", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := PreviewWorkflow(serverDescGithub(), "stk", "bogus")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("bogus"))
	})
}

func TestGenerateWorkflowsFromServerConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("unsupported type", func(t *testing.T) {
		RegisterTestingT(t)
		sd := &api.ServerDescriptor{CiCd: api.CiCdDescriptor{Type: "gitlab"}}
		err := GenerateWorkflowsFromServerConfig(sd, "s", t.TempDir())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported CI/CD type"))
	})

	t.Run("invalid config from stub converter", func(t *testing.T) {
		RegisterTestingT(t)
		// ConvertToEnhancedConfig stub -> empty -> Validate fails on auth-token.
		err := GenerateWorkflowsFromServerConfig(serverDescGithub(), "s", t.TempDir())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid CI/CD configuration"))
	})
}

func TestSyncWorkflows_PackageLevel(t *testing.T) {
	RegisterTestingT(t)

	t.Run("unsupported type", func(t *testing.T) {
		RegisterTestingT(t)
		sd := &api.ServerDescriptor{CiCd: api.CiCdDescriptor{Type: "gitlab"}}
		err := SyncWorkflows(sd, "s", t.TempDir())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported CI/CD type"))
	})

	t.Run("auto-update disabled is a no-op", func(t *testing.T) {
		RegisterTestingT(t)
		// Stub converter yields AutoUpdate=false -> early return nil.
		Expect(SyncWorkflows(serverDescGithub(), "s", t.TempDir())).To(Succeed())
	})
}

// sanity: rendered workflows never leave unresolved Go template delimiters.
func TestRenderedWorkflows_NoUnresolvedTemplates(t *testing.T) {
	RegisterTestingT(t)

	wg := NewWorkflowGenerator(fullEnhancedConfig(), "s", "", false)
	Expect(wg.LoadTemplates()).To(Succeed())
	data := wg.prepareTemplateData()
	for _, name := range GetWorkflowTemplateNames() {
		content, err := wg.generateWorkflowContent(name, data)
		Expect(err).ToNot(HaveOccurred())
		// GitHub Actions expressions ${{ ... }} are expected; bare Go template
		// delimiters {{ that are not part of ${{ would indicate a rendering bug.
		Expect(strings.Contains(content, "{{ .")).To(BeFalse(), "template %q has unresolved field ref", name)
	}
}
