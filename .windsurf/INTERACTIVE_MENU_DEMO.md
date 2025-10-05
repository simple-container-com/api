# Interactive Provider Selection Menu

## Overview
Added an interactive menu for selecting LLM providers when using `/apikey set` without specifying a provider. This makes it easier to configure multiple providers without remembering their exact names.

## Feature Highlights

✅ **Visual Provider List** - Shows all 5 supported providers  
✅ **Configuration Status** - Indicates which providers are already configured  
✅ **Easy Selection** - Just enter a number (1-5)  
✅ **Cancel Option** - Press 'q' to cancel  
✅ **User-Friendly** - No need to remember provider names  

## Demo Flow

### First Time Setup

```bash
./bin/sc assistant chat

💬 /apikey set

📋 Select a provider to configure:

  1. OpenAI (not configured)
  2. Ollama (not configured)
  3. Anthropic (not configured)
  4. DeepSeek (not configured)
  5. Yandex ChatGPT (not configured)

Enter number (1-5) or 'q' to cancel: 1
✓ Selected: OpenAI

🔑 Enter your OpenAI API key: ****************
✅ OpenAI API key saved successfully to ~/.sc/assistant-config.json
💡 Provider 'openai' is now set as default. Use '/provider switch' to change providers.
```

### Adding Another Provider

```bash
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
🌐 Enter Ollama base URL (press Enter for http://localhost:11434): 
🤖 Enter default model (press Enter for llama2): mistral
✅ Ollama API key saved successfully to ~/.sc/assistant-config.json
💡 Provider 'ollama' is now set as default. Use '/provider switch' to change providers.
```

### Reconfiguring Existing Provider

```bash
💬 /apikey set

📋 Select a provider to configure:

  1. OpenAI ✓ (configured)
  2. Ollama ✓ (configured)
  3. Anthropic (not configured)
  4. DeepSeek (not configured)
  5. Yandex ChatGPT (not configured)

Enter number (1-5) or 'q' to cancel: 1
✓ Selected: OpenAI

🔑 Enter your OpenAI API key: ****************
✅ OpenAI API key saved successfully to ~/.sc/assistant-config.json
💡 Provider 'openai' is now set as default. Use '/provider switch' to change providers.
```

### Canceling Selection

```bash
💬 /apikey set

📋 Select a provider to configure:

  1. OpenAI ✓ (configured)
  2. Ollama ✓ (configured)
  3. Anthropic (not configured)
  4. DeepSeek (not configured)
  5. Yandex ChatGPT (not configured)

Enter number (1-5) or 'q' to cancel: q
❌ No provider selected
```

## Alternative: Direct Provider Specification

You can still specify the provider directly if you prefer:

```bash
# Skip the menu by specifying provider
💬 /apikey set anthropic
🔑 Enter your Anthropic API key: ****************
✅ Anthropic API key saved successfully
```

## Implementation Details

### Function: `selectProvider()`

**Location:** `pkg/assistant/chat/commands.go`

**Features:**
- Lists all 5 supported providers
- Shows configuration status with visual indicators:
  - `✓ (configured)` - Green checkmark for configured providers
  - `(not configured)` - Yellow text for unconfigured providers
- Accepts numeric input (1-5)
- Supports cancellation with 'q', 'Q', 'quit', or 'cancel'
- Returns selected provider name or empty string if cancelled

**Code Flow:**
```go
1. Load current config
2. Get list of all valid providers
3. Check which providers are already configured
4. Display numbered menu with status indicators
5. Read user input
6. Validate selection
7. Return selected provider
```

### Integration with `/apikey set`

**Logic:**
```go
if len(args) > 1 {
    // Provider specified directly: /apikey set openai
    provider = args[1]
} else if action == "set" {
    // No provider specified: /apikey set
    // Show interactive menu
    provider = selectProvider(cfg)
} else {
    // For other actions, use default provider
    provider = cfg.GetDefaultProvider()
}
```

## Benefits

### User Experience
- **Easier Discovery** - Users see all available providers
- **Visual Feedback** - Know which providers are configured
- **Less Typing** - Just enter a number instead of provider name
- **Mistake Prevention** - No typos in provider names

### Developer Experience
- **Extensible** - Easy to add new providers to the list
- **Maintainable** - Centralized provider list
- **Consistent** - Same menu for all users

## Testing

```bash
# Build
go build -o bin/sc ./cmd/sc

# Test interactive menu
./bin/sc assistant chat

# Test menu with no providers configured
💬 /apikey set
# Should show all providers as "not configured"

# Configure one provider
# Select option 1 (OpenAI)

# Test menu again
💬 /apikey set
# Should show OpenAI as "✓ (configured)"

# Test cancellation
💬 /apikey set
# Enter 'q'
# Should cancel without error

# Test direct specification (should skip menu)
💬 /apikey set ollama
# Should NOT show menu, go directly to API key prompt

# Test invalid selection
💬 /apikey set
# Enter '99' or 'invalid'
# Should show error message
```

## Future Enhancements

Possible improvements:
- Arrow key navigation (up/down to select)
- Search/filter providers by name
- Show provider descriptions in menu
- Highlight default provider in menu
- Show last used date for configured providers
- Bulk configuration wizard
- Import/export provider configs
