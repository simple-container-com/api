# Multi-Provider LLM Support

## Overview
Extended the Simple Container AI Assistant to support multiple LLM providers with persistent configuration storage. Users can now configure and switch between OpenAI, Ollama, Anthropic, DeepSeek, and Yandex ChatGPT.

## Supported Providers

### 1. OpenAI
- **Models**: GPT-3.5-turbo, GPT-4, GPT-4-turbo, etc.
- **API Key**: Required (starts with `sk-`)
- **Use Case**: Production-ready, high-quality responses

### 2. Ollama
- **Models**: Llama2, Mistral, CodeLlama, etc.
- **API Key**: Optional (can be "none" for local)
- **Base URL**: Configurable (default: `http://localhost:11434`)
- **Use Case**: Local/self-hosted models, privacy-focused

### 3. Anthropic
- **Models**: Claude, Claude-2, Claude-instant
- **API Key**: Required
- **Use Case**: Alternative to OpenAI with different strengths

### 4. DeepSeek
- **Models**: DeepSeek-coder, DeepSeek-chat
- **API Key**: Required
- **Use Case**: Code-focused models

### 5. Yandex ChatGPT
- **Models**: Yandex GPT models
- **API Key**: Required
- **Use Case**: Russian language support, regional compliance

## What Was Implemented

### 1. Enhanced Config Package (`pkg/assistant/config/config.go`)

**New Types:**
```go
type ProviderConfig struct {
    APIKey  string `json:"api_key,omitempty"`
    BaseURL string `json:"base_url,omitempty"` // For Ollama/custom endpoints
    Model   string `json:"model,omitempty"`    // Default model
}

type Config struct {
    DefaultProvider string                    `json:"default_provider,omitempty"`
    Providers       map[string]ProviderConfig `json:"providers,omitempty"`
    Preferences     map[string]string         `json:"preferences,omitempty"`
}
```

**New Methods:**
- `SetProviderConfig(provider, config)` - Store provider configuration
- `GetProviderConfig(provider)` - Retrieve provider configuration
- `DeleteProviderConfig(provider)` - Remove provider configuration
- `HasProviderConfig(provider)` - Check if provider is configured
- `GetDefaultProvider()` - Get current default provider
- `SetDefaultProvider(provider)` - Change default provider
- `ListProviders()` - List all configured providers
- `IsValidProvider(provider)` - Validate provider name
- `GetProviderDisplayName(provider)` - Get user-friendly name

**Backward Compatibility:**
- Old `openai_api_key` field automatically migrated to new format
- Existing configs work without changes

### 2. Updated Chat Commands (`pkg/assistant/chat/commands.go`)

**Enhanced `/apikey` Command:**
```bash
/apikey set [provider]          # Set API key for provider (interactive menu if no provider)
/apikey delete [provider]       # Delete API key
/apikey status [provider]       # Show status (all or specific)
```

**Features:**
- **Interactive provider selection menu** - Shows all providers with configuration status
- Provider-specific prompts
- Ollama: Asks for base URL and default model
- Shows all configured providers when no provider specified
- Displays provider-specific info (base URL, model)
- Visual indicators for configured vs unconfigured providers

**New `/provider` Command:**
```bash
/provider list                  # List all configured providers
/provider switch [provider]     # Switch default provider (interactive if no provider specified)
/provider info [provider]       # Show provider configuration
```

**Features:**
- **Interactive provider switching** - Shows menu of configured providers only
- Shows current default provider with ⭐ marker
- Validates provider before switching
- Displays full provider configuration
- Auto-selects if only one provider configured

### 3. Updated Assistant CLI (`pkg/cmd/cmd_assistant/assistant.go`)

**Provider Display on Start:**
- Shows which provider is being used
- Displays provider-specific info (base URL, model)
- Example:
  ```
  ✅ Using stored Ollama API key
     Base URL: http://localhost:11434
     Model: llama2
  ```

**Auto-Detection:**
- Loads default provider from config
- Falls back to OpenAI if no default set
- Shows helpful provider information

### 4. Documentation (`docs/docs/ai-assistant/commands.md`)

**New Sections:**
- Multi-Provider Support overview
- Provider Management commands
- Provider-specific examples
- Provider switching workflow

## Usage Examples

### Initial Setup - Multiple Providers

```bash
# Start chat
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

🔑 Enter your OpenAI API key: sk-...
✅ OpenAI API key saved successfully

# Configure another provider using the menu
💬 /apikey set

📋 Select a provider to configure:

  1. OpenAI ✓ (configured)
  2. Ollama (not configured)
  3. Anthropic (not configured)
  4. DeepSeek (not configured)
  5. Yandex ChatGPT (not configured)

Enter number (1-5) or 'q' to cancel: 2
✓ Selected: Ollama

🔑 Enter your Ollama API key: none
🌐 Enter Ollama base URL: http://localhost:11434
🤖 Enter default model: llama2
✅ Ollama API key saved successfully

# Or configure directly without menu
💬 /apikey set anthropic
🔑 Enter your Anthropic API key: sk-ant-...
✅ Anthropic API key saved successfully
```

### View All Providers

```bash
💬 /apikey status
📋 Configured Providers:

  • OpenAI (default): sk-proj...xyz
  • Ollama: none...xyz
    Base URL: http://localhost:11434
  • Anthropic: sk-ant...xyz

Stored in: ~/.sc/assistant-config.json
```

### Switch Between Providers

```bash
# List available providers
💬 /provider list
📋 Available Providers:

  • OpenAI ⭐ (current)
  • Ollama
  • Anthropic

# Switch using interactive menu (recommended)
💬 /provider switch

📋 Select a provider to switch to:

  1. OpenAI ⭐ (current)
  2. Ollama
  3. Anthropic

Enter number (1-3) or 'q' to cancel: 2
✓ Selected: Ollama
✅ Switched to Ollama
💡 Restart the chat session to use the new provider

# Or switch directly
💬 /provider switch ollama
✅ Switched to Ollama

# Exit and restart
exit

./bin/sc assistant chat
✅ Using stored Ollama API key
   Base URL: http://localhost:11434
   Model: llama2
```

### View Provider Info

```bash
# Current provider
💬 /provider info
ℹ️  Ollama Configuration:

  Provider: ollama
  API Key: none...xyz
  Base URL: http://localhost:11434
  Default Model: llama2

# Specific provider
💬 /provider info openai
ℹ️  OpenAI Configuration:

  Provider: openai
  API Key: sk-proj...xyz
```

## Config File Format

**Location:** `~/.sc/assistant-config.json`

**Example:**
```json
{
  "default_provider": "ollama",
  "providers": {
    "openai": {
      "api_key": "sk-proj-..."
    },
    "ollama": {
      "api_key": "none",
      "base_url": "http://localhost:11434",
      "model": "llama2"
    },
    "anthropic": {
      "api_key": "sk-ant-..."
    },
    "deepseek": {
      "api_key": "sk-..."
    },
    "yandex": {
      "api_key": "..."
    }
  },
  "preferences": {}
}
```

## Benefits

✅ **Multi-Provider Support** - Use OpenAI, Ollama, Anthropic, DeepSeek, or Yandex  
✅ **Easy Switching** - Change providers with a single command  
✅ **Local Models** - Support for Ollama local/self-hosted models  
✅ **Provider-Specific Config** - Base URLs, models, etc.  
✅ **Backward Compatible** - Existing configs automatically migrated  
✅ **Secure Storage** - All keys stored with 0600 permissions  
✅ **Visual Feedback** - Shows current provider on startup  
✅ **Flexible** - Can configure multiple providers and switch anytime  

## Use Cases

### 1. Development with Local Models
```bash
# Use Ollama for development (free, private)
/provider switch ollama
```

### 2. Production with OpenAI
```bash
# Switch to OpenAI for production (high quality)
/provider switch openai
```

### 3. Cost Optimization
```bash
# Use DeepSeek for code tasks (cost-effective)
/provider switch deepseek
```

### 4. Regional Compliance
```bash
# Use Yandex for Russian market
/provider switch yandex
```

### 5. A/B Testing
```bash
# Compare responses from different providers
/provider switch openai
# Ask question, note response
/provider switch anthropic
# Ask same question, compare
```

## Testing

```bash
# Clean start
rm -f ~/.sc/assistant-config.json

# Build
go build -o bin/sc ./cmd/sc

# Test multi-provider setup
./bin/sc assistant chat

# Configure multiple providers
/apikey set openai
/apikey set ollama
/apikey set anthropic

# Test commands
/apikey status
/provider list
/provider switch ollama
/provider info

# Exit and verify provider loads
exit
./bin/sc assistant chat
# Should show: ✅ Using stored Ollama API key
```

## Next Steps

Future enhancements could include:
- Auto-detect Ollama installation
- Provider-specific model selection UI
- Cost tracking per provider
- Response quality comparison
- Provider failover/fallback
- Custom provider support
