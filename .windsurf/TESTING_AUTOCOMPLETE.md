# Testing Guide - Autocomplete & History Features

## Quick Test Checklist

### ✅ Basic Functionality
- [ ] Chat starts successfully
- [ ] Prompt displays correctly
- [ ] Can type regular text
- [ ] Enter submits input
- [ ] Exit commands work

### ✅ Autocomplete
- [ ] Tab completes single match
- [ ] Tab shows suggestions for multiple matches
- [ ] Autocomplete works with command names
- [ ] Autocomplete works with aliases
- [ ] Invalid input doesn't crash

### ✅ History
- [ ] Up arrow shows previous command
- [ ] Down arrow shows next command
- [ ] History wraps at boundaries
- [ ] Duplicates are filtered
- [ ] Empty commands not saved

### ✅ Commands
- [ ] `/history` shows command list
- [ ] `/history clear` clears history
- [ ] All existing commands still work

## Detailed Test Scenarios

### 1. Autocomplete - Single Match

```bash
# Start chat
./bin/sc assistant chat

# Test single match completion
💬 /hel<Tab>
Expected: Autocompletes to "/help"

💬 /ana<Tab>
Expected: Autocompletes to "/analyze"

💬 /cle<Tab>
Expected: Autocompletes to "/clear"
```

**Pass Criteria:**
- Command completes automatically
- Cursor at end of completed command
- Can press Enter to execute

### 2. Autocomplete - Multiple Matches

```bash
💬 /s<Tab>
Expected: Shows suggestions:
Suggestions:
  /search - Search Simple Container documentation
  /setup - Generate configuration files
  /switch - Switch between dev and devops modes
  /status - Show current session status

💬 /p<Tab>
Expected: Shows:
Suggestions:
  /provider - Manage LLM provider settings
```

**Pass Criteria:**
- All matching commands shown
- Descriptions displayed
- Original input preserved
- Can continue typing or Tab again

### 3. Autocomplete - Aliases

```bash
💬 /h<Tab>
Expected: Shows:
Suggestions:
  /help - Show available commands and usage
  /history - Show command history

💬 /s<Tab>
Expected: Includes both /search and /status
```

**Pass Criteria:**
- Aliases work like main commands
- All matches shown

### 4. History Navigation - Basic

```bash
# Execute some commands
💬 /help
💬 /status
💬 /provider list

# Navigate back
💬 <↑>
Expected: Shows "/provider list"

💬 <↑>
Expected: Shows "/status"

💬 <↑>
Expected: Shows "/help"

# Navigate forward
💬 <↓>
Expected: Shows "/status"

💬 <↓>
Expected: Shows "/provider list"
```

**Pass Criteria:**
- Commands appear in reverse order
- Can navigate back and forth
- Original input cleared when navigating

### 5. History Navigation - Boundaries

```bash
# At oldest command
💬 <↑><↑><↑>
Expected: Stops at first command

# Try going further
💬 <↑>
Expected: Stays at first command

# At newest
💬 <↓><↓><↓>
Expected: Clears input (beyond last command)
```

**Pass Criteria:**
- Doesn't crash at boundaries
- Stops gracefully
- Clear input when past last command

### 6. History - Duplicate Filtering

```bash
💬 /help
💬 /help
💬 /help

# Check history
💬 /history
Expected: Only one "/help" entry
```

**Pass Criteria:**
- Consecutive duplicates filtered
- Only unique sequence stored

### 7. History Command

```bash
# Execute various commands
💬 /help
💬 /apikey status
💬 /provider list
💬 /search postgres
💬 /analyze

# View history
💬 /history
Expected:
📜 Command History (5 commands):

  1. /help
  2. /apikey status
  3. /provider list
  4. /search postgres
  5. /analyze

💡 Tip: Use ↑/↓ arrow keys to navigate history, Tab for autocomplete
```

**Pass Criteria:**
- All commands listed
- Numbered correctly
- Tip message shown

### 8. History Clear

```bash
💬 /history
Expected: Shows commands

💬 /history clear
Expected: ✅ Command history cleared

💬 /history
Expected: No command history yet

💬 <↑>
Expected: No effect (empty history)
```

**Pass Criteria:**
- History cleared successfully
- Arrow keys don't crash
- Can rebuild history

### 9. Keyboard Shortcuts

```bash
# Ctrl+C
💬 /hel<Ctrl+C>
Expected: 
👋 Goodbye! Happy coding with Simple Container!
(Exits gracefully)

# Ctrl+D (empty input)
💬 <Ctrl+D>
Expected: Exits like "exit" command

# Backspace
💬 /help<Backspace><Backspace>
Expected: Shows "/hel"
```

**Pass Criteria:**
- Ctrl+C exits gracefully
- Ctrl+D works when input empty
- Backspace deletes characters

### 10. Mixed Usage

```bash
# Type, autocomplete, edit
💬 /hel<Tab>
Expected: "/help"

💬 <Backspace><Backspace>lp
Expected: "/help"

# History + autocomplete
💬 /apikey status
💬 <↑>
Expected: "/apikey status"

💬 <Backspace><Backspace><Backspace><Backspace><Backspace><Backspace>set<Tab>
Expected: "/apikey set"
```

**Pass Criteria:**
- Features work together
- No conflicts
- Smooth experience

### 11. Non-Command Input

```bash
💬 Hello, how are you?
Expected: Normal chat response (no autocomplete)

💬 What is Simple Container?
Expected: Normal chat response
```

**Pass Criteria:**
- Regular chat still works
- Autocomplete only for commands
- No interference

### 12. Edge Cases

```bash
# Empty Tab
💬 <Tab>
Expected: No crash, no action

# Invalid command
💬 /xyz<Tab>
Expected: No suggestions, no crash

# Very long input
💬 /help this is a very long command with lots of text
Expected: Handles gracefully

# Special characters
💬 /help @#$%
Expected: Handles gracefully
```

**Pass Criteria:**
- No crashes
- Graceful handling
- Clear error messages if needed

## Performance Tests

### Response Time
```bash
# Autocomplete should be instant
💬 /h<Tab>
Expected: < 50ms completion

# History navigation should be instant
💬 <↑>
Expected: < 50ms display
```

### Memory Usage
```bash
# Fill history to max (100 commands)
# Execute 100+ commands
# Check memory doesn't grow unbounded
```

### Large History
```bash
# Execute 100 commands
# Test navigation still fast
# Test /history command
Expected: Shows last 20, mentions total
```

## Regression Tests

### Existing Functionality
- [ ] `/help` works
- [ ] `/search` works
- [ ] `/analyze` works
- [ ] `/setup` works
- [ ] `/apikey` works
- [ ] `/provider` works
- [ ] `/status` works
- [ ] `/clear` works
- [ ] Regular chat works
- [ ] LLM responses work

## Platform-Specific Tests

### macOS
- [ ] Terminal.app works
- [ ] iTerm2 works
- [ ] Arrow keys work
- [ ] Tab works
- [ ] Ctrl+C works

### Linux
- [ ] gnome-terminal works
- [ ] konsole works
- [ ] xterm works
- [ ] All shortcuts work

### Non-TTY (Fallback)
```bash
# Pipe input
echo "/help" | ./bin/sc assistant chat
Expected: Falls back to simple input

# Redirect
./bin/sc assistant chat < commands.txt
Expected: Works without terminal features
```

## Automated Test Script

```bash
#!/bin/bash
# test-autocomplete.sh

echo "Testing autocomplete and history features..."

# Build
echo "Building..."
go build -o bin/sc ./cmd/sc || exit 1

# Test 1: Help autocomplete
echo "Test 1: Autocomplete /help"
# (Manual test required - terminal interaction)

# Test 2: History command
echo "Test 2: History command"
# (Manual test required - terminal interaction)

echo "Manual testing required for full coverage"
echo "See TESTING_AUTOCOMPLETE.md for test scenarios"
```

## Bug Report Template

If you find issues, report with:

```
**Bug:** [Brief description]

**Steps to Reproduce:**
1. Start chat
2. Type /h
3. Press Tab
4. [What happened]

**Expected:** [What should happen]
**Actual:** [What actually happened]

**Environment:**
- OS: macOS 14.0
- Terminal: iTerm2
- Go version: 1.24.0

**Additional Context:**
[Screenshots, logs, etc.]
```

## Success Criteria

All tests should pass with:
- ✅ No crashes
- ✅ Correct behavior
- ✅ Good performance
- ✅ Graceful error handling
- ✅ Backward compatibility

## Known Limitations

1. **Windows** - May have different terminal behavior
2. **Non-TTY** - Falls back to simple input (expected)
3. **History** - Not persisted across sessions (by design)
4. **Inline Suggestions** - May not work in all terminals

## Next Steps After Testing

1. Fix any bugs found
2. Optimize performance if needed
3. Add more tests
4. Consider persistent history
5. Add more keyboard shortcuts
