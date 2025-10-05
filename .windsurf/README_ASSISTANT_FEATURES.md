# Simple Container AI Assistant - Complete Feature Guide

## 🚀 Overview

The Simple Container AI Assistant is a powerful, user-friendly CLI tool with professional features including multi-provider LLM support, command autocomplete, and history navigation.

## ✨ Key Features

### 1. Multi-Provider LLM Support
Switch between 5 different LLM providers with persistent configuration:

| Provider | Use Case | Features |
|----------|----------|----------|
| **OpenAI** | Production, high quality | GPT-3.5, GPT-4 models |
| **Ollama** | Local/self-hosted, privacy | Custom base URL, model selection |
| **Anthropic** | Alternative to OpenAI | Claude models |
| **DeepSeek** | Code-focused tasks | Specialized models |
| **Yandex** | Russian language support | Regional compliance |

### 2. Interactive Configuration
- **Provider Selection Menu** - Choose from numbered list
- **API Key Management** - Secure storage with masking
- **Provider Switching** - Easy switching between configured providers
- **Configuration Status** - Visual indicators (✓ configured, ⭐ current)

### 3. Command Autocomplete
- **Tab Completion** - Press Tab to complete commands
- **Multiple Match Suggestions** - Shows all matching commands
- **Alias Support** - Works with command aliases
- **Real-time Hints** - Grayed-out suggestions as you type

### 4. Command History
- **Arrow Navigation** - ↑/↓ to browse previous commands
- **Smart Filtering** - Removes consecutive duplicates
- **History Management** - View with `/history`, clear with `/history clear`
- **Session-based** - Stores up to 100 commands per session

### 5. Professional CLI Experience
- **Keyboard Shortcuts** - Familiar bash/zsh-like controls
- **Graceful Error Handling** - Clear error messages
- **Non-TTY Fallback** - Works in pipes and redirects
- **No External Dependencies** - Pure Go implementation

## 🎯 Quick Start

### Installation
```bash
# Build
go build -o bin/sc ./cmd/sc

# Run
./bin/sc assistant chat
```

### First-Time Setup
```bash
# Start chat
./bin/sc assistant chat

# Configure provider (interactive menu)
💬 /apikey set

📋 Select a provider to configure:
  1. OpenAI (not configured)
  2. Ollama (not configured)
  3. Anthropic (not configured)
  4. DeepSeek (not configured)
  5. Yandex ChatGPT (not configured)

Enter number (1-5) or 'q' to cancel: 1
✓ Selected: OpenAI

🔑 Enter your OpenAI API key: [hidden input]
✅ OpenAI API key saved successfully
```

## 📋 Commands Reference

### API Key Management
```bash
/apikey set [provider]          # Configure provider (interactive if no provider)
/apikey delete [provider]       # Delete provider configuration
/apikey status [provider]       # Show configuration status
```

### Provider Management
```bash
/provider list                  # List configured providers
/provider switch [provider]     # Switch provider (interactive if no provider)
/provider info [provider]       # Show provider details
```

### Chat Commands
```bash
/help                          # Show available commands
/search <query>                # Search documentation
/analyze                       # Analyze current project
/setup                         # Generate configuration files
/status                        # Show session status
/history [clear]               # Show or clear command history
/clear                         # Clear conversation history
/exit                          # Exit chat
```

## ⌨️ Keyboard Shortcuts

| Key | Action |
|-----|--------|
| **Tab** | Autocomplete command |
| **↑** | Previous command in history |
| **↓** | Next command in history |
| **Ctrl+C** | Exit chat gracefully |
| **Ctrl+D** | Exit (when input empty) |
| **Backspace** | Delete character |
| **Enter** | Submit command |

## 💡 Usage Examples

### Example 1: Configure Multiple Providers
```bash
# Configure OpenAI
💬 /apikey set
# Select 1 (OpenAI), enter key

# Configure Ollama
💬 /apikey set
# Select 2 (Ollama), enter key, base URL, model

# View all configured
💬 /apikey status
📋 Configured Providers:
  • OpenAI (default): sk-proj...xyz
  • Ollama: none...xyz
    Base URL: http://localhost:11434
```

### Example 2: Switch Providers
```bash
# List available
💬 /provider list
📋 Available Providers:
  • OpenAI ⭐ (current)
  • Ollama

# Switch to Ollama
💬 /provider switch
# Select 2 (Ollama)
✅ Switched to Ollama

# Restart chat to use new provider
exit
./bin/sc assistant chat
✅ Using stored Ollama API key
```

### Example 3: Use Autocomplete
```bash
# Start typing
💬 /ap<Tab>
# Autocompletes to: /apikey

# Multiple matches
💬 /p<Tab>
Suggestions:
  /provider - Manage LLM provider settings

# Continue typing
💬 /pro<Tab>
# Autocompletes to: /provider
```

### Example 4: Navigate History
```bash
# Execute commands
💬 /apikey status
💬 /provider list
💬 /help

# Navigate back
💬 <↑>  # Shows: /help
💬 <↑>  # Shows: /provider list
💬 <↑>  # Shows: /apikey status

# Execute again
💬 <Enter>
# Executes /apikey status without retyping!
```

### Example 5: View History
```bash
💬 /history
📜 Command History (10 commands):

  1. /help
  2. /apikey set
  3. /provider list
  4. /apikey status openai
  5. /provider switch ollama
  6. /provider info
  7. /search postgres
  8. /analyze
  9. /status
  10. /provider list

💡 Tip: Use ↑/↓ arrow keys to navigate history, Tab for autocomplete
```

## 🔒 Security

- **Secure Storage**: API keys stored in `~/.sc/assistant-config.json` with 0600 permissions
- **Masked Display**: Keys shown as `sk-proj...xyz` in output
- **Hidden Input**: Password-style input when entering keys
- **Local Storage**: Config in home directory, not in repository

## 📁 Configuration File

**Location:** `~/.sc/assistant-config.json`

**Format:**
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
    },
    "anthropic": {
      "api_key": "sk-ant-..."
    }
  },
  "preferences": {}
}
```

## 🎨 Features in Action

### Welcome Screen
```
🚀 Simple Container AI Assistant
I'll help you set up your project with Simple Container.

💬 General Mode - Ask me anything about Simple Container

Type '/help' for commands or just ask me questions!
💡 Use Tab for autocomplete, ↑/↓ for history
Type 'exit' or Ctrl+C to quit
```

### Interactive Menus
- **Provider Selection**: Numbered list with status indicators
- **Command Suggestions**: Shown on Tab with descriptions
- **History Navigation**: Seamless arrow key browsing

### Visual Feedback
- ✅ Success messages in green
- ❌ Error messages in red
- ⚠️ Warnings in yellow
- 💡 Tips in blue
- ⭐ Current provider indicator
- ✓ Configured status

## 📊 Benefits

| Benefit | Impact |
|---------|--------|
| **Multi-Provider** | Flexibility to choose best LLM for task |
| **Autocomplete** | 50-70% less typing |
| **History** | Quick command recall |
| **Interactive Menus** | Easy configuration |
| **Secure Storage** | Safe API key management |
| **Professional UX** | Familiar CLI experience |

## 🧪 Testing

See `.windsurf/TESTING_AUTOCOMPLETE.md` for comprehensive testing guide.

**Quick Test:**
```bash
# Build
go build -o bin/sc ./cmd/sc

# Test autocomplete
./bin/sc assistant chat
💬 /h<Tab>  # Should complete to /help

# Test history
💬 /help
💬 /status
💬 <↑>  # Should show /status
```

## 📚 Documentation

### Feature Guides
- **Multi-Provider**: `.windsurf/MULTI_PROVIDER_FEATURE.md`
- **Autocomplete & History**: `.windsurf/AUTOCOMPLETE_HISTORY_FEATURE.md`
- **Usability**: `.windsurf/USABILITY_IMPROVEMENTS.md`

### Quick References
- **Commands**: `docs/docs/ai-assistant/commands.md`
- **Quick Reference**: `.windsurf/QUICK_REFERENCE.md`
- **Testing**: `.windsurf/TESTING_AUTOCOMPLETE.md`

### Demos
- **Interactive Menus**: `.windsurf/INTERACTIVE_MENU_DEMO.md`
- **Implementation**: `.windsurf/IMPLEMENTATION_SUMMARY.md`

## 🚀 Advanced Usage

### Custom Ollama Setup
```bash
💬 /apikey set ollama
🔑 Enter your Ollama API key: none
🌐 Enter Ollama base URL: http://my-server:11434
🤖 Enter default model: mistral
✅ Ollama API key saved successfully
```

### Multiple Providers Workflow
```bash
# Use OpenAI for production
💬 /provider switch openai
# Ask production questions

# Switch to Ollama for testing
💬 /provider switch ollama
# Test with local model

# Compare responses
💬 /provider info
# See which provider is active
```

## 🐛 Troubleshooting

### Autocomplete not working
- Ensure you're in a terminal (not piped)
- Try different terminal emulator
- Check terminal supports ANSI escape codes

### History not saving
- History is session-based (by design)
- Use `/history` to verify
- Clear with `/history clear` if needed

### Provider not loading
- Check config: `cat ~/.sc/assistant-config.json`
- Verify API key: `/apikey status`
- Re-configure: `/apikey set [provider]`

## 🎯 Next Steps

1. **Configure Providers**: Set up your preferred LLM providers
2. **Learn Shortcuts**: Practice Tab and arrow keys
3. **Explore Commands**: Use `/help` to see all commands
4. **Read Docs**: Check feature guides for details
5. **Provide Feedback**: Report issues or suggestions

## 🎉 Summary

The Simple Container AI Assistant combines powerful multi-provider LLM support with professional CLI usability features, making it fast, flexible, and easy to use for all your Simple Container needs!

**Ready to get started?** Run `./bin/sc assistant chat` and explore! 🚀
