package github

// Embedded workflow templates for GitHub Actions generation

const deployTemplate = `name: Deploy {{ .Organization.Name }} {{ .StackName }}

on:
  push:
    branches: [{{ if .DefaultBranch }}{{ .DefaultBranch }}{{ else }}main{{ end }}]
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to deploy to'
        required: true
        type: choice
        options: [{{ envNamesExcluding .Environments "preview" }}]
        default: '{{ .DefaultEnvironment }}'
      skip_validation:
        description: 'Skip validation checks'
        required: false
        type: boolean
        default: false

concurrency:
  group: {{ if .Execution.Concurrency.Group }}{{ .Execution.Concurrency.Group }}{{ else }}deploy-{{ .StackName }}-${{ "{{" }} github.ref {{ "}}" }}{{ end }}
  cancel-in-progress: {{ .Execution.Concurrency.CancelInProgress }}

permissions:
  contents: read
  deployments: write
  pull-requests: write
  statuses: write

env:
  STACK_NAME: "{{ .StackName }}"

jobs:
{{- range $envName, $env := .Environments }}
{{- if ne $env.Type "preview" }}
  deploy-{{ $envName }}:
    name: Deploy to {{ title $envName }}
    {{- if $env.Protection }}
    environment: 
      name: {{ $envName }}
      {{- if $env.Reviewers }}
      protection_rules:
        required_reviewers: {{ $env.Reviewers | yamlList }}
      {{- end }}
    {{- end }}
    runs-on: {{ if $env.Runners }}{{ index $env.Runners 0 }}{{ else }}ubuntu-latest{{ end }}
    timeout-minutes: {{ if $.Execution.DefaultTimeout }}{{ timeoutMinutes $.Execution.DefaultTimeout }}{{ else }}30{{ end }}
    {{- if or (and (eq $.DefaultBranch "main") (not $env.AutoDeploy)) (eq $env.Type "production") }}
    if: ${{ "{{" }} github.event_name == 'workflow_dispatch' && github.event.inputs.environment == '{{ $envName }}' {{ "}}" }}
    {{- else if $env.AutoDeploy }}
    if: ${{ "{{" }} github.ref == 'refs/heads/{{ $.DefaultBranch }}' {{ "}}" }}
    {{- end }}
    
    steps:
      - name: Deploy {{ $.StackName }} to {{ $envName }}
        uses: {{ if index $.CustomActions "deploy" }}{{ index $.CustomActions "deploy" }}{{ else }}{{ defaultAction "deploy" $.SCVersion }}{{ end }}
        with:
          stack-name: "${{ "{{" }} env.STACK_NAME {{ "}}" }}"
          environment: "{{ $envName }}"
          sc-config: ${{ "{{" }} secrets.SC_CONFIG {{ "}}" }}
          {{- if $env.DeployFlags }}
          sc-deploy-flags: "{{ $env.DeployFlags | join " " }}"
          {{- end }}
          {{- if $env.ValidationCmd }}
          validation-command: |
            {{ $env.ValidationCmd | indent 12 }}
          {{- end }}
          # GitHub context for repository cloning
          token: ${{ "{{" }} secrets.GITHUB_TOKEN {{ "}}" }}
          repository: ${{ "{{" }} github.repository {{ "}}" }}
          ref: ${{ "{{" }} github.sha {{ "}}" }}
          cc-on-start: "{{ $.Notifications.CCOnStart }}"
          
      {{- if $.Validation.Required }}
      - name: Run validation tests
        if: ${{ "{{" }} !github.event.inputs.skip_validation {{ "}}" }}
        run: |
          {{- if index $.Validation.Commands $envName }}
          {{ index $.Validation.Commands $envName }}
          {{- else }}
          echo "No validation commands configured for {{ $envName }}"
          {{- end }}
      {{- end }}
      
      {{- if $.Validation.HealthChecks }}
      - name: Health check
        run: |
          echo "Running health checks..."
          {{- range $path, $description := $.Validation.HealthChecks }}
          echo "Checking {{ $description }}"
          curl -f "https://{{ $envName }}-api.{{ $.Organization.Name }}.com{{ $path }}" || exit 1
          {{- end }}
      {{- end }}

{{- end }}
{{- end }}`

const destroyTemplate = `name: Destroy {{ .Organization.Name }} {{ .StackName }}

on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to destroy'
        required: true
        type: choice
        options: [{{ envNamesExcluding .Environments "preview" }}]
      confirmation:
        description: 'Type DESTROY to confirm'
        required: true
        type: string
      auto_confirm:
        description: 'Skip confirmation prompts'
        required: false
        type: boolean
        default: false
      skip_backup:
        description: 'Skip backup creation'
        required: false
        type: boolean
        default: false

concurrency:
  group: destroy-{{ .StackName }}-${{ "{{" }} github.event.inputs.environment {{ "}}" }}
  cancel-in-progress: false

permissions:
  contents: read
  deployments: write
  pull-requests: write

env:
  STACK_NAME: "{{ .StackName }}"

jobs:
  validate-destroy:
    name: Validate Destruction Request
    runs-on: ubuntu-latest
    outputs:
      environment: ${{ "{{" }} steps.validate.outputs.environment {{ "}}" }}
      confirmed: ${{ "{{" }} steps.validate.outputs.confirmed {{ "}}" }}
    steps:
      - name: Validate destruction request
        id: validate
        run: |
          CONFIRMATION="${{ "{{" }} github.event.inputs.confirmation {{ "}}" }}"
          ENVIRONMENT="${{ "{{" }} github.event.inputs.environment {{ "}}" }}"
          
          if [[ "$CONFIRMATION" != "DESTROY" ]]; then
            echo "❌ Invalid confirmation. Must type 'DESTROY' exactly."
            exit 1
          fi
          
          {{- range $envName, $env := .Environments }}
          {{- if $env.Protection }}
          if [[ "$ENVIRONMENT" == "{{ $envName }}" ]]; then
            echo "⚠️ Attempting to destroy protected environment: {{ $envName }}"
            echo "This requires additional verification."
          fi
          {{- end }}
          {{- end }}
          
          echo "environment=$ENVIRONMENT" >> $GITHUB_OUTPUT
          echo "confirmed=true" >> $GITHUB_OUTPUT
          echo "✅ Destruction request validated"

  destroy-stack:
    name: Destroy Stack
    needs: validate-destroy
    {{- $hasProtectedEnvs := false }}
    {{- range $envName, $env := .Environments }}
    {{- if $env.Protection }}{{ $hasProtectedEnvs = true }}{{- end }}
    {{- end }}
    {{- if $hasProtectedEnvs }}
    environment: ${{ "{{" }} needs.validate-destroy.outputs.environment {{ "}}" }}
    {{- end }}
    runs-on: {{ if .Environments }}{{ $firstEnv := "" }}{{ range $name, $env := .Environments }}{{ if eq $firstEnv "" }}{{ $firstEnv = $name }}{{ if $env.Runners }}{{ index $env.Runners 0 }}{{ else }}ubuntu-latest{{ end }}{{ end }}{{ end }}{{ else }}ubuntu-latest{{ end }}
    timeout-minutes: {{ if .Execution.DefaultTimeout }}{{ timeoutMinutes .Execution.DefaultTimeout }}{{ else }}30{{ end }}
    
    steps:
      - name: Destroy {{ .StackName }}
        uses: {{ if index .CustomActions "destroy-client" }}{{ index .CustomActions "destroy-client" }}{{ else }}{{ defaultAction "destroy" .SCVersion }}{{ end }}
        with:
          stack-name: "${{ "{{" }} env.STACK_NAME {{ "}}" }}"
          environment: "${{ "{{" }} needs.validate-destroy.outputs.environment {{ "}}" }}"
          sc-config: ${{ "{{" }} secrets.SC_CONFIG {{ "}}" }}
          auto-confirm: ${{ "{{" }} github.event.inputs.auto_confirm {{ "}}" }}
          skip-backup: ${{ "{{" }} github.event.inputs.skip_backup {{ "}}" }}
          # GitHub context for repository cloning
          token: ${{ "{{" }} secrets.GITHUB_TOKEN {{ "}}" }}
          repository: ${{ "{{" }} github.repository {{ "}}" }}
          ref: ${{ "{{" }} github.sha {{ "}}" }}

`

const destroyParentTemplate = `name: Destroy {{ .Organization.Name }} Infrastructure

on:
  workflow_dispatch:
    inputs:
      confirmation:
        description: 'Type DESTROY-INFRASTRUCTURE to confirm'
        required: true
        type: string
      auto_confirm:
        description: 'Skip confirmation prompts'
        required: false
        type: boolean
        default: false
      skip_backup:
        description: 'Skip infrastructure backup'
        required: false
        type: boolean
        default: false

concurrency:
  group: destroy-infrastructure-{{ .StackName }}
  cancel-in-progress: false

permissions:
  contents: read
  deployments: write
  pull-requests: write

env:
  STACK_NAME: "{{ .StackName }}"

jobs:
  validate-destroy:
    name: Validate Infrastructure Destruction
    runs-on: ubuntu-latest
    outputs:
      confirmed: ${{ "{{" }} steps.validate.outputs.confirmed {{ "}}" }}
    steps:
      - name: Validate destruction request
        id: validate
        run: |
          CONFIRMATION="${{ "{{" }} github.event.inputs.confirmation {{ "}}" }}"
          
          if [[ "$CONFIRMATION" != "DESTROY-INFRASTRUCTURE" ]]; then
            echo "❌ Invalid confirmation. Please type 'DESTROY-INFRASTRUCTURE' to confirm."
            exit 1
          fi
          
          echo "⚠️  WARNING: This will destroy the entire infrastructure stack!"
          echo "⚠️  This action affects all dependent applications and services."
          echo "⚠️  Make sure all client applications are properly backed up."
          
          echo "confirmed=true" >> $GITHUB_OUTPUT
          echo "✅ Infrastructure destruction request validated"

  destroy-infrastructure:
    name: Destroy Infrastructure Stack
    needs: validate-destroy
    environment: infrastructure
    runs-on: {{ if .Environments }}{{ $firstEnv := "" }}{{ range $name, $env := .Environments }}{{ if eq $firstEnv "" }}{{ $firstEnv = $name }}{{ if $env.Runners }}{{ index $env.Runners 0 }}{{ else }}ubuntu-latest{{ end }}{{ end }}{{ end }}{{ else }}ubuntu-latest{{ end }}
    timeout-minutes: {{ if .Execution.DefaultTimeout }}{{ timeoutMinutes .Execution.DefaultTimeout }}{{ else }}60{{ end }}
    
    steps:
      - name: Destroy Parent Stack
        uses: {{ if index .CustomActions "destroy" }}{{ index .CustomActions "destroy" }}{{ else }}{{ defaultAction "destroy-parent" .SCVersion }}{{ end }}
        with:
          stack-name: "${{ "{{" }} env.STACK_NAME {{ "}}" }}"
          sc-config: ${{ "{{" }} secrets.SC_CONFIG {{ "}}" }}
          auto-confirm: ${{ "{{" }} github.event.inputs.auto_confirm {{ "}}" }}
          skip-backup: ${{ "{{" }} github.event.inputs.skip_backup {{ "}}" }}
          notify-on-completion: "true"
          # GitHub context for repository cloning
          token: ${{ "{{" }} secrets.GITHUB_TOKEN {{ "}}" }}
          repository: ${{ "{{" }} github.repository {{ "}}" }}
          ref: ${{ "{{" }} github.sha {{ "}}" }}
          # Notification webhooks automatically configured from SC secrets.yaml
          # No individual GitHub repository secrets needed - SC_CONFIG provides all secrets

`

const provisionTemplate = `name: Provision {{ .Organization.Name }} Infrastructure

on:
  push:
    branches: [{{ .Organization.DefaultBranch }}]
    paths:
      - '.sc/stacks/**'
      - 'server.yaml'
      - '*.yaml'
      - '*.yml'
  workflow_dispatch:
    inputs:
      dry_run:
        description: 'Dry run (preview changes only)'
        required: false
        type: boolean
        default: true
      skip_tests:
        description: 'Skip infrastructure tests'
        required: false
        type: boolean
        default: false

concurrency:
  group: provision-infrastructure
  cancel-in-progress: false

permissions:
  contents: read
  deployments: write
  pull-requests: write

env:
  STACK_NAME: "{{ .StackName }}"

jobs:
  provision-infrastructure:
    name: Provision Infrastructure
    environment: infrastructure
    runs-on: {{ if .Environments }}{{ $firstEnv := "" }}{{ range $name, $env := .Environments }}{{ if eq $firstEnv "" }}{{ $firstEnv = $name }}{{ if $env.Runners }}{{ index $env.Runners 0 }}{{ else }}ubuntu-latest{{ end }}{{ end }}{{ end }}{{ else }}ubuntu-latest{{ end }}
    timeout-minutes: {{ if .Execution.DefaultTimeout }}{{ timeoutMinutes .Execution.DefaultTimeout }}{{ else }}30{{ end }}
    
    steps:
      - name: Provision Parent Stack
        uses: {{ if index .CustomActions "provision" }}{{ index .CustomActions "provision" }}{{ else }}{{ defaultAction "provision" .SCVersion }}{{ end }}
        with:
          stack-name: "${{ "{{" }} env.STACK_NAME {{ "}}" }}"
          sc-config: ${{ "{{" }} secrets.SC_CONFIG {{ "}}" }}
          # For push triggers: dry-run=false (deploy), for manual dispatch: use input (default=true)
          dry-run: ${{ "{{" }} github.event_name == 'push' && 'false' || github.event.inputs.dry_run || 'true' {{ "}}" }}
          skip-tests: ${{ "{{" }} github.event.inputs.skip_tests || 'false' {{ "}}" }}
          notify-on-completion: "true"
          # GitHub context for repository cloning
          token: ${{ "{{" }} secrets.GITHUB_TOKEN {{ "}}" }}
          repository: ${{ "{{" }} github.repository {{ "}}" }}
          ref: ${{ "{{" }} github.sha {{ "}}" }}
          # Notification webhooks automatically configured from SC secrets.yaml
          # No individual GitHub repository secrets needed - SC_CONFIG provides all secrets

  test-infrastructure:
    name: Test Infrastructure
    needs: provision-infrastructure
    runs-on: ubuntu-latest
    # Run tests unless explicitly skipped or in dry-run mode
    # For push: always run tests, for manual dispatch: respect user inputs
    if: ${{ "{{" }} success() && (github.event_name == 'push' || (!github.event.inputs.skip_tests && !github.event.inputs.dry_run)) {{ "}}" }}
    
    steps:
      - name: Run infrastructure tests
        run: |
          echo "🧪 Running infrastructure tests..."
          {{- if .Validation.TestSuites }}
          {{- range .Validation.TestSuites }}
          echo "Running {{ . }} test suite..."
          # {{ . }} test commands would go here
          {{- end }}
          {{- else }}
          echo "No test suites configured"
          {{- end }}
          
      {{- if .Validation.HealthChecks }}
      - name: Health check infrastructure
        run: |
          echo "🏥 Checking infrastructure health..."
          {{- range $path, $description := .Validation.HealthChecks }}
          echo "Checking {{ $description }}"
          # Health check for {{ $path }} would go here
          {{- end }}
      {{- end }}`

const prPreviewTemplate = `name: PR Preview - {{ .Organization.Name }} {{ .StackName }}

on:
  pull_request:
    types: [opened, synchronize, labeled, unlabeled]
  pull_request_target:
    types: [closed]

concurrency:
  group: pr-preview-${{ "{{" }} github.event.pull_request.number {{ "}}" }}
  cancel-in-progress: true

permissions:
  contents: read
  deployments: write
  pull-requests: write
  statuses: write

env:
  STACK_NAME: "{{ .StackName }}"
  PR_NUMBER: ${{ "{{" }} github.event.pull_request.number {{ "}}" }}

jobs:
  check-deploy-label:
    name: Check Deploy Label
    runs-on: ubuntu-latest
    outputs:
      should-deploy: ${{ "{{" }} steps.check.outputs.should-deploy {{ "}}" }}
      preview-enabled: ${{ "{{" }} steps.check.outputs.preview-enabled {{ "}}" }}
    steps:
      - name: Check for deploy label
        id: check
        run: |
          {{- $previewEnv := "" }}
          {{- range $envName, $env := .Environments }}
          {{- if eq $env.Type "preview" }}{{ $previewEnv = $envName }}{{ end }}
          {{- end }}
          
          LABELS="${{ "{{" }} toJson(github.event.pull_request.labels.*.name) {{ "}}" }}"
          DEPLOY_LABEL="{{- if ne $previewEnv "" }}{{ (index .Environments $previewEnv).PRPreview.LabelTrigger }}{{- else }}deploy-preview{{- end }}"
          
          if echo "$LABELS" | jq -r '.[]' | grep -q "^$DEPLOY_LABEL$"; then
            echo "should-deploy=true" >> $GITHUB_OUTPUT
            echo "✅ Deploy label found: $DEPLOY_LABEL"
          else
            echo "should-deploy=false" >> $GITHUB_OUTPUT
            echo "❌ Deploy label not found. Add '$DEPLOY_LABEL' to deploy."
          fi
          
          {{- if ne $previewEnv "" }}
          echo "preview-enabled=true" >> $GITHUB_OUTPUT
          {{- else }}
          echo "preview-enabled=false" >> $GITHUB_OUTPUT
          {{- end }}

  deploy-preview:
    name: Deploy PR Preview
    needs: check-deploy-label
    if: ${{ "{{" }} github.event.action != 'closed' && needs.check-deploy-label.outputs.should-deploy == 'true' && needs.check-deploy-label.outputs.preview-enabled == 'true' {{ "}}" }}
    runs-on: {{ if .Environments }}{{ $firstEnv := "" }}{{ range $name, $env := .Environments }}{{ if eq $firstEnv "" }}{{ $firstEnv = $name }}{{ if $env.Runners }}{{ index $env.Runners 0 }}{{ else }}ubuntu-latest{{ end }}{{ end }}{{ end }}{{ else }}ubuntu-latest{{ end }}
    timeout-minutes: {{ if .Execution.DefaultTimeout }}{{ timeoutMinutes .Execution.DefaultTimeout }}{{ else }}30{{ end }}
    
    steps:
      - name: Deploy PR Preview
        uses: {{ if index .CustomActions "deploy" }}{{ index .CustomActions "deploy" }}{{ else }}{{ defaultAction "deploy" .SCVersion }}{{ end }}
        with:
          stack-name: "${{ "{{" }} env.STACK_NAME {{ "}}" }}"
          environment: "preview"
          sc-config: ${{ "{{" }} secrets.SC_CONFIG {{ "}}" }}
          pr-preview: "true"
          pr-number: "${{ "{{" }} env.PR_NUMBER {{ "}}" }}"
          {{- $previewEnv := "" }}
          {{- range $envName, $env := .Environments }}
          {{- if eq $env.Type "preview" }}{{ $previewEnv = $envName }}{{ end }}
          {{- end }}
          {{- if and (ne $previewEnv "") (index .Environments $previewEnv).PRPreview.DomainBase }}
          preview-domain-base: "{{ (index .Environments $previewEnv).PRPreview.DomainBase }}"
          {{- else }}
          preview-domain-base: "preview.{{ .Organization.Name }}.com"
          {{- end }}
          {{- if and (ne $previewEnv "") (index .Environments $previewEnv).ValidationCmd }}
          validation-command: |
            {{ (index .Environments $previewEnv).ValidationCmd | indent 12 }}
          {{- end }}
          # GitHub context for repository cloning
          token: ${{ "{{" }} secrets.GITHUB_TOKEN {{ "}}" }}
          repository: ${{ "{{" }} github.repository {{ "}}" }}
          ref: ${{ "{{" }} github.sha {{ "}}" }}
          
      - name: Comment PR with preview URL
        uses: actions/github-script@v7
        with:
          script: |
            const previewUrl = process.env.PR_NUMBER 
              ? 'https://pr' + process.env.PR_NUMBER + '-preview.{{ .Organization.Name }}.com'
              : 'Preview URL not available';
              
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: '🚀 **PR Preview Deployed**\n\n📱 **Preview URL:** ' + previewUrl + '\n\n_This preview will be automatically cleaned up when the PR is closed._'
            });

  destroy-preview:
    name: Destroy PR Preview
    if: ${{ "{{" }} github.event.action == 'closed' {{ "}}" }}
    runs-on: ubuntu-latest
    timeout-minutes: 15
    
    steps:
      - name: Destroy PR Preview
        uses: {{ if index .CustomActions "destroy-client" }}{{ index .CustomActions "destroy-client" }}{{ else }}{{ defaultAction "destroy" .SCVersion }}{{ end }}
        with:
          stack-name: "${{ "{{" }} env.STACK_NAME {{ "}}" }}"
          environment: "preview"
          sc-config: ${{ "{{" }} secrets.SC_CONFIG {{ "}}" }}
          pr-preview: "true"
          pr-number: "${{ "{{" }} env.PR_NUMBER {{ "}}" }}"
          auto-confirm: "true"
          skip-backup: "true"
          # GitHub context for repository cloning
          token: ${{ "{{" }} secrets.GITHUB_TOKEN {{ "}}" }}
          repository: ${{ "{{" }} github.repository {{ "}}" }}
          ref: ${{ "{{" }} github.sha {{ "}}" }}
          
      - name: Comment PR cleanup
        uses: actions/github-script@v7
        with:
          script: |
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: '🧹 **PR Preview Cleaned Up**\n\nThe preview environment has been automatically destroyed.'
            });`
