# Commands Reference

Complete reference for all Simple Container AI Assistant commands, organized by mode and functionality.

## 📋 Command Overview

| Command                                      | Mode      | Description                      |
|----------------------------------------------|-----------|----------------------------------|
| [`sc assistant dev`](#developer-commands)    | Developer | Application-focused commands     |
| [`sc assistant devops`](#devops-commands)    | DevOps    | Infrastructure-focused commands  |
| [`sc assistant search`](#search-commands)    | Both      | Documentation search             |
| [`sc assistant chat`](#interactive-commands) | Both      | Interactive assistance (Phase 3) |
| [`sc assistant mcp`](#mcp-commands)          | Both      | MCP server management            |

## 🧑‍💻 Developer Commands

### `sc assistant dev setup`

Generate application configuration files based on project analysis.

**Usage:**
```bash
sc assistant dev setup [options]
```

**Options:**
| Flag | Description | Default |
|------|-------------|---------|
| `--interactive, -i` | Interactive mode with prompts | `false` |
| `--env <environment>` | Target environment | `staging` |
| `--parent <stack>` | Parent stack name | `infrastructure` |
| `--skip-analysis` | Skip automatic project analysis | `false` |
| `--skip-dockerfile` | Skip Dockerfile generation | `false` |
| `--skip-compose` | Skip docker-compose.yaml generation | `false` |
| `--language <lang>` | Override detected language | Auto-detected |
| `--framework <framework>` | Override detected framework | Auto-detected |
| `--cloud <provider>` | Target cloud provider | From parent stack |
| `--output-dir <dir>` | Output directory | `.sc/stacks/<project-name>/` |

**Examples:**
```bash
# Basic setup with auto-detection
sc assistant dev setup

# Interactive setup with prompts
sc assistant dev setup --interactive

# Target production environment
sc assistant dev setup --env production

# Skip Docker files, only generate Simple Container config
sc assistant dev setup --skip-dockerfile --skip-compose

# Override detected tech stack
sc assistant dev setup --language python --framework django

# Use specific parent infrastructure
sc assistant dev setup --parent my-company-infra --env staging
```

**Generated Files:**
- `client.yaml` - Simple Container application configuration
- `docker-compose.yaml` - Local development environment
- `Dockerfile` - Container image definition (if not exists)

### `sc assistant dev analyze`

Analyze project structure and detect technology stack.

**Usage:**
```bash
sc assistant dev analyze [options]
```

**Options:**
| Flag | Description | Default |
|------|-------------|---------|
| `--detailed` | Show detailed analysis output | `false` |
| `--path <directory>` | Project path to analyze | `.` |
| `--output <file>` | Export analysis to JSON file | Console output |
| `--format <format>` | Output format (json, yaml, table) | `table` |

**Examples:**
```bash
# Basic project analysis
sc assistant dev analyze

# Detailed analysis with recommendations
sc assistant dev analyze --detailed

# Analyze specific directory
sc assistant dev analyze --path ./services/api

# Export analysis to file
sc assistant dev analyze --output analysis.json --format json

# Analyze and show in table format
sc assistant dev analyze --format table
```

**Output Example:**
```
🔍 Project Analysis Results

📊 Technology Stack:
   Language:     Node.js (18.x)
   Framework:    Express.js
   Architecture: REST API
   Confidence:   95%

📦 Dependencies:
   ✅ express      ^4.18.0   (Web framework)
   ✅ pg           ^8.8.0    (PostgreSQL client)
   ✅ redis        ^4.3.0    (Cache client)
   ✅ jsonwebtoken ^8.5.1    (Authentication)

🎯 Recommendations:
   🔹 Add PostgreSQL database resource
   🔹 Add Redis cache resource
   🔹 Configure health check endpoint
   🔹 Add request logging middleware
```

## 🛠️ DevOps Commands

### `sc assistant devops setup`

Set up infrastructure configuration with interactive wizard.

**Usage:**
```bash
sc assistant devops setup [options]
```

**Options:**
| Flag | Description | Default |
|------|-------------|---------|
| `--interactive, -i` | Interactive wizard mode | `true` |
| `--cloud <provider>` | Cloud provider (aws, gcp, k8s) | Interactive selection |
| `--envs <environments>` | Comma-separated environments | `staging,production` |
| `--resources <types>` | Comma-separated resource types | Interactive selection |
| `--templates <names>` | Template names to create | Interactive selection |
| `--prefix <name>` | Resource name prefix | Project name |
| `--region <region>` | Default cloud region | Provider default |
| `--output-dir <dir>` | Output directory | `.sc/stacks/infrastructure/` |

**Examples:**
```bash
# Interactive wizard (recommended for first-time setup)
sc assistant devops setup

# AWS setup with specific environments
sc assistant devops setup --cloud aws --envs staging,production,testing

# Quick setup with common resources
sc assistant devops setup --cloud gcp --resources database,cache,storage

# Multi-cloud setup
sc assistant devops setup --cloud aws,gcp --primary aws

# Custom prefix and region
sc assistant devops setup --prefix mycompany --region us-west-2
```

**Generated Files:**
- `server.yaml` - Infrastructure resources and templates
- `secrets.yaml` - Authentication and sensitive configuration
- `cfg.default.yaml` - Default Simple Container settings

### `sc assistant devops resources`

Manage shared infrastructure resources.

**Usage:**
```bash
sc assistant devops resources [action] [options]
```

**Actions:**
| Action | Description |
|--------|-------------|
| `--list` | List available resource types |
| `--add <type>` | Add resource to infrastructure |
| `--remove <name>` | Remove resource from infrastructure |
| `--update <name>` | Update existing resource |
| `--template <name>` | Create resource template |

**Options:**
| Flag | Description | Default |
|------|-------------|---------|
| `--cloud <provider>` | Filter by cloud provider | All providers |
| `--env <environment>` | Target environment | All environments |
| `--interactive, -i` | Interactive resource configuration | `false` |
| `--copy-from <env>` | Copy resource from another environment | None |
| `--scale-up` | Increase resource capacity | Current settings |
| `--scale-down` | Decrease resource capacity | Current settings |

**Examples:**
```bash
# List all available resource types
sc assistant devops resources --list

# List AWS-specific resources
sc assistant devops resources --list --cloud aws

# Add PostgreSQL database interactively
sc assistant devops resources --add postgres --interactive

# Add Redis cache to staging environment
sc assistant devops resources --add redis --env staging

# Copy production database config to staging (with smaller instance)
sc assistant devops resources --add postgres --env staging --copy-from production --scale-down

# Update existing database to larger instance
sc assistant devops resources --update postgres-db --scale-up
```

### `sc assistant devops secrets`

Manage authentication credentials and secrets.

**Usage:**
```bash
sc assistant devops secrets [action] [options]
```

**Actions:**
| Action | Description |
|--------|-------------|
| `--init` | Initialize secrets configuration |
| `--auth <provider>` | Configure cloud provider authentication |
| `--generate <names>` | Generate random secrets |
| `--import-from <source>` | Import secrets from external system |
| `--rotate <names>` | Rotate existing secrets |

**Options:**
| Flag | Description | Default |
|------|-------------|---------|
| `--cloud <provider>` | Cloud provider for authentication | Current provider |
| `--interactive, -i` | Interactive secret entry | `false` |
| `--length <n>` | Generated secret length | `32` |
| `--export-to <file>` | Export secrets template | Console |

**Examples:**
```bash
# Initialize secrets for AWS
sc assistant devops secrets --init --cloud aws

# Configure AWS credentials interactively
sc assistant devops secrets --auth aws --interactive

# Generate application secrets
sc assistant devops secrets --generate jwt-secret,api-key,encryption-key

# Import secrets from AWS Secrets Manager
sc assistant devops secrets --import-from aws-secrets-manager

# Rotate database passwords
sc assistant devops secrets --rotate staging-db-password,prod-db-password
```

## 🔍 Search Commands

### `sc assistant search`

Search Simple Container documentation using semantic similarity.

**Usage:**
```bash
sc assistant search <query> [options]
```

**Options:**
| Flag | Description | Default |
|------|-------------|---------|
| `--limit <n>` | Maximum results to return | `5` |
| `--type <type>` | Document type (docs, examples, schemas) | `all` |
| `--provider <name>` | Filter by cloud provider | `all` |
| `--format <format>` | Output format (table, json, detailed) | `table` |
| `--threshold <n>` | Minimum similarity score (0.0-1.0) | `0.7` |

**Examples:**
```bash
# Basic documentation search
sc assistant search "PostgreSQL database configuration"

# Limit results and search only examples
sc assistant search "Node.js deployment" --limit 10 --type examples

# Search for AWS-specific resources
sc assistant search "S3 bucket setup" --provider aws

# High-precision search with JSON output
sc assistant search "GKE autopilot" --threshold 0.9 --format json

# Search for schema information
sc assistant search "client.yaml structure" --type schemas
```

**Output Example:**
```
🔍 Search Results for: "PostgreSQL database configuration"

┌───┬─────────────────────────┬──────────┬───────────┬─────────────────────────────────────┐
│ # │ Document                │ Type     │ Similarity│ Preview                             │
├───┼─────────────────────────┼──────────┼───────────┼─────────────────────────────────────┤
│ 1 │ supported-resources.md  │ docs     │ 0.894     │ PostgreSQL database configuration.  │
│   │                         │          │           │ Use aws-rds-postgres for AWS...    │
├───┼─────────────────────────┼──────────┼───────────┼─────────────────────────────────────┤
│ 2 │ postgres-example.md     │ examples │ 0.847     │ Complete PostgreSQL setup example  │
│   │                         │          │           │ with environment variables...       │
├───┼─────────────────────────┼──────────┼───────────┼─────────────────────────────────────┤
│ 3 │ rds-postgres.json       │ schemas  │ 0.798     │ JSON schema for AWS RDS PostgreSQL  │
│   │                         │          │           │ resource configuration...           │
└───┴─────────────────────────┴──────────┴───────────┴─────────────────────────────────────┘

Found 3 relevant documents in 89ms
```

## 🤖 Interactive Commands

### `sc assistant chat`

Start interactive chat session with AI assistant (Phase 3).

**Usage:**
```bash
sc assistant chat [options]
```

**Options:**
| Flag | Description | Default |
|------|-------------|---------|
| `--mode <mode>` | Chat mode (developer, devops, general) | Auto-detect |
| `--context <path>` | Project context directory | `.` |
| `--model <name>` | LLM model to use | `gpt-3.5-turbo` |
| `--save-session` | Save conversation history | `false` |
| `--load-session <file>` | Load previous conversation | None |

**Examples:**
```bash
# Start general chat (uses stored API key if available)
sc assistant chat

# Developer-focused chat with project context
sc assistant chat --mode developer --context .

# DevOps chat for infrastructure questions
sc assistant chat --mode devops

# Use specific API key for this session
sc assistant chat --openai-key sk-your-key-here

# Save conversation for later reference
sc assistant chat --save-session conversation.json
```

**Note:** If you have stored your OpenAI API key using `/apikey set` in a previous session, it will be automatically loaded. You can manage your stored API key using the `/apikey` command within the chat interface.

**Chat Commands:**
- `/help` - Show available chat commands
- `/search <query>` - Search documentation inline
- `/analyze` - Analyze current project
- `/setup` - Generate configuration files
- `/switch <mode>` - Switch between dev and devops modes
- `/clear` - Clear conversation history
- `/status` - Show current session status
- `/apikey <set|delete|status> [provider]` - Manage LLM provider API keys
- `/provider <list|switch|info> [provider]` - Manage LLM providers
- `/exit` - Exit chat session

### Multi-Provider Support

The assistant supports multiple LLM providers with persistent API key storage:

**Supported Providers:**
- **OpenAI** - GPT models (gpt-3.5-turbo, gpt-4, etc.)
- **Ollama** - Local or self-hosted models (llama2, mistral, etc.)
- **Anthropic** - Claude models
- **DeepSeek** - DeepSeek models
- **Yandex** - Yandex ChatGPT

### API Key Management

**Setting an API Key:**
```bash
# Interactive provider selection (recommended)
💬 /apikey set

📋 Select a provider to configure:

  1. OpenAI ✓ (configured)
  2. Ollama (not configured)
  3. Anthropic (not configured)
  4. DeepSeek (not configured)
  5. Yandex ChatGPT (not configured)

Enter number (1-5) or 'q' to cancel: 2
✓ Selected: Ollama

🔑 Enter your Ollama API key: [hidden input]
🌐 Enter Ollama base URL (press Enter for http://localhost:11434): 
🤖 Enter default model (press Enter for llama2): 
✅ Ollama API key saved successfully

# Or specify provider directly
💬 /apikey set openai
🔑 Enter your OpenAI API key: [hidden input]
✅ OpenAI API key saved successfully

# Set Anthropic API key
💬 /apikey set anthropic
🔑 Enter your Anthropic API key: [hidden input]
✅ Anthropic API key saved successfully
```

**Checking API Key Status:**
```bash
# Show all configured providers
💬 /apikey status
📋 Configured Providers:

  • OpenAI (default): sk-proj...xyz
  • Ollama: none...xyz
    Base URL: http://localhost:11434

Stored in: /Users/username/.sc/assistant-config.json

# Show specific provider
💬 /apikey status openai
✅ OpenAI API key is configured: sk-proj...xyz
   Stored in: /Users/username/.sc/assistant-config.json
```

**Deleting Stored API Key:**
```bash
# Delete default provider's API key
💬 /apikey delete
✅ OpenAI API key deleted successfully

# Delete specific provider's API key
💬 /apikey delete ollama
✅ Ollama API key deleted successfully
```

### Provider Management

**List Configured Providers:**
```bash
💬 /provider list
📋 Available Providers:

  • OpenAI ⭐ (current)
  • Ollama
  • Anthropic

Use '/provider switch <provider>' to change the default provider
```

**Switch Between Providers:**
```bash
# Interactive selection (recommended)
💬 /provider switch

📋 Select a provider to switch to:

  1. OpenAI ⭐ (current)
  2. Ollama
  3. Anthropic

Enter number (1-3) or 'q' to cancel: 2
✓ Selected: Ollama
✅ Switched to Ollama
💡 Restart the chat session to use the new provider

# Or specify provider directly
💬 /provider switch ollama
✅ Switched to Ollama
💡 Restart the chat session to use the new provider
```

**View Provider Info:**
```bash
💬 /provider info
ℹ️  OpenAI Configuration:

  Provider: openai
  API Key: sk-proj...xyz

# View specific provider
💬 /provider info ollama
ℹ️  Ollama Configuration:

  Provider: ollama
  API Key: none...xyz
  Base URL: http://localhost:11434
  Default Model: llama2
```

**API Key Priority:**

The assistant checks for API keys in the following order:
1. Command line flag: `--openai-key sk-...`
2. Environment variable: `OPENAI_API_KEY`
3. Stored config (default provider): `~/.sc/assistant-config.json`
4. Interactive prompt (with option to save)

**First-Time Setup:**

When you start chat without an API key, you'll be prompted to enter it and asked if you want to save it:

```bash
sc assistant chat
⚠️  OpenAI API key not found
...
🔑 Enter your OpenAI API key: [hidden input]
💾 Save this API key for future sessions? (Y/n): y
✅ API key saved to ~/.sc/assistant-config.json
```

**Provider Display on Start:**

When you start a chat session, the assistant shows which provider is being used:

```bash
sc assistant chat
✅ Using stored OpenAI API key

🚀 Simple Container AI Assistant
...
```

**Security Notes:**
- API keys are stored in `~/.sc/assistant-config.json` with restricted permissions (0600)
- Keys are masked when displayed (e.g., `sk-proj...xyz`)
- The config file is stored in your home directory and not committed to version control

## 🌐 MCP Commands

### `sc assistant mcp`

Start MCP (Model Context Protocol) server for external tool integration.

**Usage:**
```bash
sc assistant mcp [options]
```

**Options:**
| Flag | Description | Default |
|------|-------------|---------|
| `--host <address>` | Server host address | `localhost` |
| `--port <number>` | Server port | `9999` |
| `--verbose, -v` | Verbose logging | `false` |
| `--config <file>` | Configuration file path | `.sc/mcp-config.yaml` |
| `--auth` | Enable API key authentication | `false` |
| `--cors-origin <origins>` | CORS allowed origins | `*` |
| `--rate-limit <n>` | Requests per minute | `unlimited` |

**Examples:**
```bash
# Start MCP server on default port
sc assistant mcp

# Start on specific host and port
sc assistant mcp --host 0.0.0.0 --port 8080

# Start with authentication and rate limiting
sc assistant mcp --auth --rate-limit 120

# Start with custom configuration
sc assistant mcp --config /etc/simple-container/mcp.yaml

# Start with verbose logging for debugging
sc assistant mcp --verbose
```

**Server Endpoints:**
- `GET /health` - Health check
- `GET /capabilities` - Server capabilities
- `POST /mcp` - JSON-RPC MCP endpoint
- `GET /metrics` - Prometheus metrics
- `GET /debug/memory` - Memory usage info

## 🔧 Global Options

All `sc assistant` commands support these global options:

| Flag | Description | Default |
|------|-------------|---------|
| `--help, -h` | Show command help | |
| `--version` | Show assistant version | |
| `--config <file>` | Configuration file | `.sc/cfg.default.yaml` |
| `--profile <name>` | Simple Container profile | `default` |
| `--verbose, -v` | Verbose output | `false` |
| `--quiet, -q` | Minimal output | `false` |
| `--no-color` | Disable colored output | `false` |
| `--json` | JSON output format | `false` |

## 📊 Command Chaining

Commands can be chained together for complex workflows:

```bash
# Analyze project, then set up configuration
sc assistant dev analyze --output analysis.json && \
sc assistant dev setup --skip-analysis

# Set up infrastructure, then start MCP server
sc assistant devops setup && \
sc assistant mcp --port 9999

# Search for examples, then generate configuration
sc assistant search "Express.js API setup" --limit 1 && \
sc assistant dev setup --framework express
```

## 🔍 Exit Codes

All commands return standard exit codes:

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error |
| `2` | Command line argument error |
| `3` | Configuration error |
| `4` | Project analysis error |
| `5` | File generation error |
| `6` | Network/MCP server error |

## 💡 Tips and Best Practices

### **Command Aliases**
Add to your shell configuration:
```bash
# ~/.bashrc or ~/.zshrc
alias sca='sc assistant'
alias scad='sc assistant dev'
alias scao='sc assistant devops'
alias scas='sc assistant search'
```

### **Configuration Management**
```bash
# Set default options in environment
export SC_ASSISTANT_MODE=developer
export SC_ASSISTANT_CLOUD=aws
export SC_ASSISTANT_ENV=staging

# Use configuration file for complex setups
cat > ~/.sc/assistant-config.yaml << EOF
mode: developer
defaults:
  cloud: aws
  environment: staging
  parent: infrastructure
search:
  limit: 10
  threshold: 0.8
EOF
```

### **Automation Scripts**
```bash
#!/bin/bash
# setup-new-project.sh
set -e

echo "Setting up new project with Simple Container AI Assistant"

# Analyze project
sc assistant dev analyze --detailed

# Generate configuration
sc assistant dev setup --interactive

# Start local development
docker-compose up -d

echo "Project setup complete! Run 'sc deploy -e staging' to deploy."
```

## 🔗 See Also

- **[Getting Started Guide](getting-started.md)** - Basic usage walkthrough
- **[Developer Mode Guide](developer-mode.md)** - Detailed developer workflows
- **[DevOps Mode Guide](devops-mode.md)** - Infrastructure management
- **[MCP Integration](mcp-integration.md)** - External tool integration
- **[Troubleshooting](troubleshooting.md)** - Common issues and solutions
