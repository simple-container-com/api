# Config Diff Implementation Plan

## Goal
Add a command to show configuration changes (diff) in Simple Container with YAML inheritance resolution, similar to `git diff` or GitHub diff view.

## Problem
Due to YAML inheritance (via `inherit:` directives), it's not always clear what exactly changed in the final configuration after all inheritance is applied.

## Solution Architecture

### 1. New Package: `pkg/assistant/configdiff`

**Core Components:**

```
pkg/assistant/configdiff/
├── differ.go          # Core YAML comparison logic
├── resolver.go        # Resolve inheritance to final YAML
├── formatter.go       # Diff formatting in git/github style
├── types.go           # Types for representing changes
└── differ_test.go     # Tests
```

**Key Functions:**

- `ResolveInheritedConfig(stackName, configType)` - resolves all inheritance to final YAML
- `CompareConfigs(before, after)` - compares two configurations
- `FormatDiff(changes, format)` - formats changes (unified, split, inline)
- `GetConfigSnapshot(stackName, ref)` - gets a configuration snapshot (current or from git)

### 2. Chat Commands Integration

**New Command:** `/diff`

```go
c.commands["diff"] = &ChatCommand{
    Name:        "diff",
    Description: "Show configuration changes with inheritance resolved",
    Usage:       "/diff <stack_name> [--type client|server] [--ref HEAD~1] [--format unified|split]",
    Handler:     c.handleConfigDiff,
    Args: []CommandArg{
        {Name: "stack_name", Type: "string", Required: true},
        {Name: "type", Type: "string", Default: "client"},
        {Name: "ref", Type: "string", Default: "HEAD"},
        {Name: "format", Type: "string", Default: "unified"},
    },
}
```

### 3. MCP Server Integration

**New MCP Tool:** `show_config_diff`

```json
{
  "name": "show_config_diff",
  "description": "📊 Show configuration changes with resolved inheritance (git diff style)",
  "inputSchema": {
    "type": "object",
    "properties": {
      "stack_name": {
        "type": "string",
        "description": "Stack name to show diff for"
      },
      "config_type": {
        "type": "string",
        "enum": ["client", "server"],
        "default": "client"
      },
      "compare_with": {
        "type": "string",
        "description": "Git ref to compare with (e.g., 'HEAD~1', 'main', commit hash)",
        "default": "HEAD"
      },
      "format": {
        "type": "string",
        "enum": ["unified", "split", "inline"],
        "default": "unified"
      }
    },
    "required": ["stack_name"]
  }
}
```

## Detailed Design

### Stage 1: Inheritance Resolution

type ConfigResolver struct {
    stacksMap api.StacksMap
    versionProvider ConfigVersionProvider
}

func (r *ConfigResolver) ResolveStack(stackName string) (*ResolvedConfig, error) {
    // 1. Get stack from StacksMap
    // 2. Apply ResolveInheritance() to get final state
    // 3. Serialize to YAML
    // 4. Return ResolvedConfig with metadata
}

### Stage 2: Getting Versions for Comparison

```go
type ConfigVersionProvider interface {
    GetCurrent(stackName, configType string) (*ResolvedConfig, error)
    GetFromGit(stackName, configType, gitRef string) (*ResolvedConfig, error)
    GetFromLocal(stackName, configType, filePath string) (*ResolvedConfig, error)
```

**Implementation:**
- Current version: read from `.sc/stacks/{stack}/client.yaml` or `server.yaml`
- Git version: use `git show {ref}:.sc/stacks/{stack}/client.yaml`

### Stage 3: Configuration Comparison

```go
type ConfigDiff struct {
    Added    []DiffLine
    Removed  []DiffLine
    Modified []DiffLine
    Unchanged []DiffLine
}

type DiffLine struct {
    LineNumber int
    Path       string  // YAML path (e.g., "stacks.prod.config.scale.min")
    OldValue   string
    NewValue   string
    Context    []string // Surrounding lines for context
}

func CompareYAML(before, after string) (*ConfigDiff, error) {
    // Use a library for YAML-aware diff
    // Preserve structure and comments
}
```

### Stage 4: Output Formatting

**Unified format (git diff style):**
```diff
--- .sc/stacks/myapp/client.yaml (HEAD~1)
+++ .sc/stacks/myapp/client.yaml (current)
@@ -15,7 +15,7 @@
 stacks:
   prod:
     type: cloud-compose
-    parent: infrastructure/base
+    parent: infrastructure/production
     config:
       scale:
-        min: 2
+        min: 5
```

**Split format (GitHub style, one line per change):**
```
📋 Configuration Changes: myapp/client.yaml

🔹 Environment: prod

  stacks.prod.parent
  │ infrastructure/base → infrastructure/production  ⚠️  Inheritance chain modified

  stacks.prod.config.scale.min
  │ 2 → 5  (Minimum instances increased by 150%)
```

**Inline format (most compact):**
```
stacks.prod.parent: infrastructure/base → infrastructure/production ⚠️
stacks.prod.config.scale.min: 2 → 5
```

**Compact format (one line, without stacks prefix):**
```
prod.parent: infrastructure/base → infrastructure/production ⚠️
prod.config.scale.min: 2 → 5
```

## Implementation Plan by Stages

### Stage 1: Create Base Infrastructure (2-3 hours)
- [ ] Create `pkg/assistant/configdiff` package
- [ ] Implement `ConfigResolver` for inheritance resolution
- [ ] Write tests for resolver

### Stage 2: Implement Diff Logic (3-4 hours)
- [ ] Implement `ConfigVersionProvider` with git support
- [ ] Implement YAML-aware diff (can use `go-yaml-diff` library)
- [ ] Add formatters (unified, split, inline)
- [ ] Write tests for diff

### Stage 3: Chat Integration (1-2 hours)
- [ ] Add `/diff` command to `commands_project.go`
- [ ] Implement `handleConfigDiff`
- [ ] Add error handling and validation

### Stage 4: MCP Integration (1-2 hours)
- [ ] Add `show_config_diff` tool to `server.go`
- [ ] Implement handler in `executeToolCall`
- [ ] Update MCP documentation

### Stage 5: Testing and Documentation (2-3 hours)
- [ ] E2E tests with real configurations
- [ ] Tests with various inheritance scenarios
- [ ] Update user documentation
- [ ] Add usage examples

## Technical Details

### Dependencies
- `gopkg.in/yaml.v3` - already in use
- `github.com/sergi/go-diff` - for unified diff
- Possibly: `github.com/pmezard/go-difflib` - alternative

### Edge Cases Handling
1. **Circular dependencies in inheritance** - detect and error
2. **Missing git ref** - clear error message
3. **Non-existent stack** - validation before processing
4. **Empty configurations** - proper handling
5. **Large diffs** - pagination or output limiting

### Security
- Obfuscate secrets in diff (use existing `obfuscateCredentials`)
- Don't show `secrets.yaml` content in diff
- Validate git ref to prevent injection

## Usage Examples

### Chat Interface
```bash
# Show changes in current stack's client.yaml
/diff myapp

# Compare with previous commit
/diff myapp --ref HEAD~1

# Show changes in server.yaml
/diff infrastructure --type server

# Use split format
/diff myapp --format split
```

### MCP Server
```json
{
  "method": "tools/call",
  "params": {
    "name": "show_config_diff",
    "arguments": {
      "stack_name": "myapp",
      "config_type": "client",
      "compare_with": "HEAD~1",
      "format": "unified"
    }
  }
}
```

## Future Extensions

1. **Diff between two arbitrary refs**
   ```bash
   /diff myapp --from main --to feature-branch
   ```

2. **Interactive mode with change application**
   ```bash
   /diff myapp --interactive  # Allows selecting which changes to apply
   ```

3. **Export diff to file**
   ```bash
   /diff myapp --output changes.patch
   ```

4. **Web UI visualization**
   - Integration with existing web interface
   - Color-coded change highlighting

## Risks and Mitigation

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Inheritance resolution complexity | Medium | High | Use existing `ResolveInheritance()` |
| Performance on large configs | Low | Medium | Cache resolved configs |
| Conflicts with current functionality | Low | Low | Thorough testing |
| Git not available | Medium | Medium | Fallback to current files comparison only |

## Success Criteria

- ✅ Correct resolution of all inheritance types
- ✅ Clear and readable diff output
- ✅ Support for all formats (unified, split, inline)
- ✅ Integration into chat and MCP without breaking changes
- ✅ Test coverage >80%
- ✅ Documentation and usage examples
