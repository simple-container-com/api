# Config Diff Examples

## Formats (Quick Reference)

| Format | Description | When to use |
|--------|-------------|-------------|
| **unified** | Git diff style with `+/-` | Code review; familiar to developers |
| **split** | GitHub style, one line per change | ⭐ **Recommended** - readable with explanations |
| **inline** | Compact `path: old → new` | Quick overview; many changes |
| **compact** | Shortest, without `stacks` prefix | Minimal text; scripts |

---

## Scenario: Changes in client.yaml for stack "myapp"

### Original configuration (HEAD~1)
```yaml
stacks:
  dev:
    type: cloud-compose
    parent: infrastructure/development
    config:
      domain: dev.myapp.com
      scale:
        min: 1
        max: 3
      env:
        - name: LOG_LEVEL
          value: debug
        - name: DB_POOL_SIZE
          value: "10"
      
  prod:
    type: cloud-compose
    parent: infrastructure/base
    config:
      domain: myapp.com
      scale:
        min: 2
        max: 10
      env:
        - name: LOG_LEVEL
          value: info
        - name: DB_POOL_SIZE
          value: "20"
```

### Current configuration (Working Directory)
```yaml
stacks:
  dev:
    type: cloud-compose
    parent: infrastructure/development
    config:
      domain: dev.myapp.com
      scale:
        min: 1
        max: 5
      env:
        - name: LOG_LEVEL
          value: debug
        - name: DB_POOL_SIZE
          value: "15"
        - name: CACHE_ENABLED
          value: "true"
      
  prod:
    type: cloud-compose
    parent: infrastructure/production  # Changed
    config:
      domain: myapp.com
      scale:
        min: 5  # Changed
        max: 20  # Changed
      env:
        - name: LOG_LEVEL
          value: warn  # Changed
        - name: DB_POOL_SIZE
          value: "50"  # Changed
        - name: CACHE_ENABLED  # Added
          value: "true"
```

---

## Format 1: Unified (Git Diff Style)

```bash
$ /diff myapp --format unified

📊 Configuration Diff: myapp/client.yaml
Comparing: HEAD~1 → Working Directory
Resolved with inheritance applied

--- .sc/stacks/myapp/client.yaml (HEAD~1) [resolved]
+++ .sc/stacks/myapp/client.yaml (current) [resolved]
@@ -1,7 +1,7 @@
 stacks:
   dev:
     type: cloud-compose
     parent: infrastructure/development
     config:
       domain: dev.myapp.com
       scale:
@@ -8,10 +8,11 @@
         min: 1
-        max: 3
+        max: 5
       env:
         - name: LOG_LEVEL
           value: debug
         - name: DB_POOL_SIZE
-          value: "10"
+          value: "15"
+        - name: CACHE_ENABLED
+          value: "true"
 
   prod:
@@ -19,17 +20,20 @@
     type: cloud-compose
-    parent: infrastructure/base
+    parent: infrastructure/production
     config:
       domain: myapp.com
       scale:
-        min: 2
-        max: 10
+        min: 5
+        max: 20
       env:
         - name: LOG_LEVEL
-          value: info
+          value: warn
         - name: DB_POOL_SIZE
-          value: "20"
+          value: "50"
+        - name: CACHE_ENABLED
+          value: "true"

📈 Summary:
  • 7 lines changed
  • 3 lines added
  • 0 lines removed
  • 2 environments affected: dev, prod
```

---

## Format 2: Split (GitHub Style)

```bash
$ /diff myapp --format split

📊 Configuration Diff: myapp/client.yaml
Comparing: HEAD~1 → Working Directory
Resolved with inheritance applied

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔹 Environment: dev

  stacks.dev.config.scale.max
  │ 3 → 5  (Scaling capacity increased by 67%)

  stacks.dev.config.env[1].value (DB_POOL_SIZE)
  │ "10" → "15"  (Database connection pool increased)

  stacks.dev.config.env[2] (NEW)
  │ + CACHE_ENABLED = "true"

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔹 Environment: prod

  stacks.prod.parent
  │ infrastructure/base → infrastructure/production  ⚠️  Inheritance chain modified

  stacks.prod.config.scale.min
  │ 2 → 5  (Minimum instances increased by 150%)

  stacks.prod.config.scale.max
  │ 10 → 20  (Maximum instances doubled)

  stacks.prod.config.env[0].value (LOG_LEVEL)
  │ info → warn  ⚠️  Log verbosity reduced

  stacks.prod.config.env[1].value (DB_POOL_SIZE)
  │ "20" → "50"  (Database connection pool increased by 150%)

  stacks.prod.config.env[2] (NEW)
  │ + CACHE_ENABLED = "true"

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📈 Summary:
  • 7 changes total
  • 2 additions
  • 0 deletions
  • 5 modifications
  
⚠️  Warnings:
  • Parent stack changed in prod - verify inheritance chain
  • Log level reduced in prod - may affect debugging
```

---

## Format 3: Inline (Compact) - Most compact

```bash
$ /diff myapp --format inline

📊 Configuration Diff: myapp/client.yaml
Comparing: HEAD~1 → Working Directory

🔹 dev:
  stacks.dev.config.scale.max: 3 → 5
  stacks.dev.config.env[1].value: "10" → "15"
  stacks.dev.config.env[2]: + CACHE_ENABLED="true"

🔹 prod:
  stacks.prod.parent: infrastructure/base → infrastructure/production ⚠️
  stacks.prod.config.scale.min: 2 → 5
  stacks.prod.config.scale.max: 10 → 20
  stacks.prod.config.env[0].value: info → warn ⚠️
  stacks.prod.config.env[1].value: "20" → "50"
  stacks.prod.config.env[2]: + CACHE_ENABLED="true"

📈 7 changes | 2 additions | 0 deletions
```

---

## Format 4: Compact (One line per change)

```bash
$ /diff myapp --format compact

📊 myapp/client.yaml (HEAD~1 → current) - 7 changes

  dev.config.scale.max: 3 → 5
  dev.config.env[1].value: "10" → "15"
  dev.config.env[2]: + CACHE_ENABLED="true"
  prod.parent: infrastructure/base → infrastructure/production ⚠️
  prod.config.scale.min: 2 → 5
  prod.config.scale.max: 10 → 20
  prod.config.env[0].value: info → warn ⚠️
  prod.config.env[1].value: "20" → "50"
  prod.config.env[2]: + CACHE_ENABLED="true"

⚠️  2 warnings
```

---

## Example with inheritance (resolved vs raw)

### Command with flag --show-inheritance

```bash
$ /diff myapp --format split --show-inheritance

📊 Configuration Diff: myapp/client.yaml
Comparing: HEAD~1 → Working Directory

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔹 stacks.prod.parent (Raw YAML)
  │ infrastructure/base → infrastructure/production

  📦 Inherited changes (resolved configuration):
  │ provisioner.config.project: my-project-dev → my-project-prod
  │ provisioner.config.region: us-central1 → us-east1
  │ 
  │ ⚠️  Parent switch affects 2 inherited properties

---

## Example with secrets (obfuscated)

```bash
$ /diff myapp --type server --format split

Configuration Diff: myapp/server.yaml
{{ ... }}
🔒 Sensitive values are obfuscated

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  secrets.values.DATABASE_URL
  │ postgres://user:••••••••@db.example.com:5432/myapp → postgres://user:••••••••@db-prod.example.com:5432/myapp
  │ (Database host changed)

  secrets.values.API_KEY
  │ AKIA•••••••••••••••• → AKIA••••••••••••••••  (Value changed - API key rotated)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔒 Note: Actual secret values are hidden for security
```

---

## Example with no changes

```bash
$ /diff myapp

📊 Configuration Diff: myapp/client.yaml
Comparing: HEAD~1 → Working Directory

✅ No changes detected

Configuration is identical to HEAD~1 after resolving inheritance.
```

---

## Example with error

```bash
$ /diff nonexistent

❌ Error: Stack 'nonexistent' not found

Available stacks:
  • myapp
  • infrastructure
  • api-gateway
  
Hint: Use '/show <stack_name>' to view stack configuration
```

---

## Interactive mode (future extension)

```bash
$ /diff myapp --interactive

📊 Configuration Diff: myapp/client.yaml
7 changes detected. Review each change:

[1/7] stacks.dev.config.scale.max: 3 → 5
      Accept this change? [y/n/s(kip all)/q(uit)]: y
      ✓ Accepted

[2/7] stacks.dev.config.env[1].value: "10" → "15"
      Accept this change? [y/n/s/q]: y
      ✓ Accepted

[3/7] stacks.prod.parent: infrastructure/base → infrastructure/production
      ⚠️  This will change inherited configuration
      Accept this change? [y/n/s/q]: n
      ✗ Rejected

...

Summary:
  ✓ 5 changes accepted
  ✗ 2 changes rejected
  
Apply accepted changes? [y/n]: y
✅ Changes applied to working directory
```

---

## Color legend (in terminal)

```
🟢 Green   - Added lines/values
🔴 Red     - Removed lines/values
🟡 Yellow  - Modified values
🔵 Blue    - Context (unchanged lines)
⚠️ Orange  - Warnings
```

---

## MCP Server Response (JSON)

```json
{
  "content": [
    {
      "type": "text",
      "text": "📊 Configuration Diff: myapp/client.yaml\nComparing: HEAD~1 → Working Directory\n\n..."
    }
  ],
  "isError": false,
  "metadata": {
    "stack_name": "myapp",
    "config_type": "client",
    "changes_count": 7,
    "additions": 2,
    "deletions": 0,
    "modifications": 5,
    "environments_affected": ["dev", "prod"],
    "warnings": [
      "Parent stack changed in prod - verify inheritance chain",
      "Log level reduced in prod - may affect debugging"
    ]
  }
}
```
