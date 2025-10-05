# Testing API Key Storage Feature

## Quick Test Steps

### 1. Clean Start (Remove any existing config)
```bash
rm -f ~/.sc/assistant-config.json
```

### 2. First Run - Save API Key
```bash
./bin/sc assistant chat

# You'll see:
# ⚠️  OpenAI API key not found
# ...
# 🔑 Enter your OpenAI API key: [paste your key - it will be hidden]
# 💾 Save this API key for future sessions? (Y/n): y
# ✅ API key saved to ~/.sc/assistant-config.json

# Then exit the chat
exit
```

### 3. Second Run - Verify Auto-Load
```bash
./bin/sc assistant chat

# You should see:
# ✅ Using stored OpenAI API key
# (No prompt for API key!)
```

### 4. Test /apikey Commands in Chat

#### Check Status
```bash
💬 /apikey status
# Should show:
# ✅ API key is configured: sk-proj...xyz
# Stored in: /Users/yourname/.sc/assistant-config.json
```

#### Update API Key
```bash
💬 /apikey set
# 🔑 Enter your OpenAI API key: [enter new key]
# ✅ OpenAI API key saved successfully
```

#### Delete API Key
```bash
💬 /apikey delete
# ✅ OpenAI API key deleted successfully

💬 /apikey status
# ❌ No API key is currently stored
# Use '/apikey set' to configure one
```

### 5. Verify Config File
```bash
# Check the config file exists and has correct permissions
ls -la ~/.sc/assistant-config.json
# Should show: -rw------- (600 permissions)

# View the content (your API key will be visible here)
cat ~/.sc/assistant-config.json
# Should show JSON with your API key
```

## Expected Behavior

✅ **First time**: Prompted for API key, option to save  
✅ **Subsequent runs**: Auto-loads from config  
✅ **`/apikey status`**: Shows masked key and location  
✅ **`/apikey set`**: Updates stored key  
✅ **`/apikey delete`**: Removes stored key  
✅ **File permissions**: 0600 (read/write owner only)  
✅ **Masked display**: Shows `sk-proj...xyz` format  

## Troubleshooting

### Issue: `/apikey status` shows "No API key stored" after entering it

**Solution**: Make sure you answered "y" or "yes" when prompted to save. If you said "n", the key was only set for that session.

### Issue: Permission denied when saving config

**Solution**: Check that `~/.sc/` directory exists and is writable:
```bash
mkdir -p ~/.sc
chmod 700 ~/.sc
```

### Issue: API key not loading on restart

**Solution**: Check the config file exists and is valid JSON:
```bash
cat ~/.sc/assistant-config.json
# Should show valid JSON with "openai_api_key" field
```

## Manual Config File Format

If needed, you can manually create/edit the config file:

```json
{
  "openai_api_key": "sk-proj-your-actual-key-here",
  "llm_provider": "openai",
  "preferences": {}
}
```

Save to: `~/.sc/assistant-config.json`  
Permissions: `chmod 600 ~/.sc/assistant-config.json`
