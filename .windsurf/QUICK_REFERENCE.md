# Quick Reference - Multi-Provider LLM Support

## 🚀 Quick Start

```bash
# Build
go build -o bin/sc ./cmd/sc

# Start chat
./bin/sc assistant chat
```

## 📋 Supported Providers

| Provider | Use Case | API Key Format |
|----------|----------|----------------|
| **OpenAI** | Production, high quality | `sk-proj-...` |
| **Ollama** | Local/self-hosted, privacy | `none` (local) |
| **Anthropic** | Alternative to OpenAI | `sk-ant-...` |
| **DeepSeek** | Code-focused | `sk-...` |
| **Yandex** | Russian language | `...` |

## 🔑 API Key Commands

```bash
# Interactive provider selection (shows menu)
/apikey set
# Shows:
# 1. OpenAI ✓ (configured)
# 2. Ollama (not configured)
# 3. Anthropic (not configured)
# 4. DeepSeek (not configured)
# 5. Yandex ChatGPT (not configured)
# Enter number (1-5) or 'q' to cancel:

# Or set API key for specific provider directly
/apikey set openai
/apikey set ollama
/apikey set anthropic
/apikey set deepseek
/apikey set yandex

# View all configured providers
/apikey status

# View specific provider
/apikey status openai

# Delete API key
/apikey delete openai
```

## 🔄 Provider Commands

```bash
# List all configured providers
/provider list

# Switch default provider (interactive menu)
/provider switch
# Shows menu of configured providers

# Or switch directly
/provider switch ollama
/provider switch openai

# View provider info
/provider info
/provider info ollama
```

## 📁 Config File

**Location:** `~/.sc/assistant-config.json`

**Structure:**
```json
{
  "default_provider": "openai",
  "providers": {
    "openai": {
      "api_key": "sk-proj-..."
    },
    "ollama": {
      "api_key": "none",
      "base_url": "http://localhost:11434",
      "model": "llama2"
    }
  }
}
```

## 🎯 Common Workflows

### Setup Multiple Providers
```bash
./bin/sc assistant chat

# Use interactive menu (recommended)
💬 /apikey set

📋 Select a provider to configure:
  1. OpenAI (not configured)
  2. Ollama (not configured)
  3. Anthropic (not configured)
  4. DeepSeek (not configured)
  5. Yandex ChatGPT (not configured)

Enter number (1-5) or 'q' to cancel: 1
✓ Selected: OpenAI
🔑 [enter key]

# Or configure directly
💬 /apikey set ollama
🔑 none
🌐 http://localhost:11434
🤖 llama2

# View all
💬 /apikey status
```

### Switch Provider
```bash
# See what's available
💬 /provider list

# Use interactive menu
💬 /provider switch

📋 Select a provider to switch to:
  1. OpenAI ⭐ (current)
  2. Ollama
  3. Anthropic

Enter number: 2
✓ Selected: Ollama
✅ Switched to Ollama and reloaded successfully!
You can continue chatting with the new provider.

# No restart needed - provider is active immediately!
💬 Hello
🤖 [Response from Ollama]
```

### Check Current Provider
```bash
# On startup, you'll see:
✅ Using stored OpenAI API key

# Or in chat:
💬 /provider info
```

## 🔒 Security

- Config file: `~/.sc/assistant-config.json`
- Permissions: `0600` (read/write owner only)
- Keys masked in display: `sk-proj...xyz`
- Secure input: Hidden when typing

## ⚡ Features

✅ Multiple provider support  
✅ Easy provider switching  
✅ Persistent configuration  
✅ Provider-specific settings (base URL, model)  
✅ Interactive menu with status indicators  
✅ Backward compatible  
✅ Secure storage  
✅ Visual feedback on startup  

## 🐛 Troubleshooting

### Provider not showing on startup
```bash
# Check config
cat ~/.sc/assistant-config.json

# Verify provider is set
./bin/sc assistant chat
💬 /provider info
```

### Switch not working
```bash
# Make sure provider is configured first
💬 /apikey set ollama

# Then switch
💬 /provider switch ollama

# Restart chat
exit
./bin/sc assistant chat
```

### Ollama connection issues
```bash
# Check Ollama is running
curl http://localhost:11434/api/tags

# Update base URL if needed
💬 /apikey set ollama
🌐 http://your-ollama-server:11434
```

## 📖 Full Documentation

See `/docs/docs/ai-assistant/commands.md` for complete documentation.
