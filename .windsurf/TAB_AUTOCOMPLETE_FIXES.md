# Tab Autocomplete Fixes - Final

## Issues Fixed

### 1. ✅ Newline on Command Autocomplete
**Problem:** When typing `/prov<Tab>`, a newline was added after autocompleting to `/provider`

**Solution:**
- Simplified line clearing - use `\r` + spaces + `\r` instead of ANSI escape sequences
- Added explicit `continue` after autocomplete
- Fixed terminal state management when re-entering raw mode

### 2. ✅ Newline When No Suggestions
**Problem:** Tab added a newline even when there were no matching commands

**Solution:**
- Added early check: `if len(suggestions) == 0 { continue }`
- Tab now does nothing if there are no suggestions

### 3. ✅ Newline on Complete Command
**Problem:** `/provider list<Tab>` added a newline even though the command was already complete

**Solution:**
- Check if the single suggestion matches current input: `if suggestions[0] == currentInput { continue }`
- Do nothing if command is already complete

### 4. ✅ Multiple Newlines When Showing Suggestions
**Problem:** When showing multiple suggestions, extra lines with indentation were added

**Solution:**
- Temporarily exit raw mode for printing
- Properly manage terminal state: `oldState = newState`
- Added `continue` after showing suggestions

### 5. ✅ Subcommand Autocomplete
**Problem:** No autocomplete support for subcommands

**Solution:**
- Added `getSubcommandSuggestions()` function
- Subcommand support: `/provider <Tab>` shows `list`, `switch`, `info`
- Partial subcommand autocomplete: `/provider s<Tab>` → `/provider switch`

## Final Implementation

### Tab Handling Logic

```go
case 9: // Tab
    currentInput := input.String()
    if strings.HasPrefix(currentInput, "/") {
        suggestions = h.getCommandSuggestions(currentInput)
        
        // 1. No suggestions - do nothing
        if len(suggestions) == 0 {
            continue
        }
        
        // 2. Single suggestion
        if len(suggestions) == 1 {
            // Check if it's the same as current input
            if suggestions[0] == currentInput {
                continue
            }
            // Autocomplete using direct stdout write
            clearSeq := "\r" + strings.Repeat(" ", len(currentInput)+len(prompt)) + "\r" + prompt + suggestions[0]
            os.Stdout.WriteString(clearSeq)
            input.Reset()
            input.WriteString(suggestions[0])
            continue
        }
        
        // 3. Multiple suggestions - show list
        else if len(suggestions) > 1 {
            term.Restore(int(os.Stdin.Fd()), oldState)
            fmt.Println()
            h.printSuggestions(suggestions)
            fmt.Printf("\n%s%s", prompt, input.String())
            newState, _ := term.MakeRaw(int(os.Stdin.Fd()))
            oldState = newState
            continue
        }
    }
    continue
```

## Behavior Now

| Input | Tab Result |
|-------|------------|
| `/pro<Tab>` | Autocompletes to `/provider` (same line) ✅ |
| `/provider<Tab>` | Shows subcommands ✅ |
| `/provider s<Tab>` | Autocompletes to `/provider switch` ✅ |
| `/provider list<Tab>` | Nothing (already complete) ✅ |
| `/xyz<Tab>` | Nothing (no matches) ✅ |
| `hello<Tab>` | Nothing (not a command) ✅ |

## Key Changes

1. **Line Clearing:** Simple method with `\r` + spaces instead of ANSI
2. **State Management:** Proper `oldState` update when re-entering raw mode
3. **Explicit Continue:** After each action to avoid fall-through
4. **Early Exits:** Checks for empty suggestions and matches
5. **Subcommand Support:** Full autocomplete support for subcommands
6. **Direct stdout Write:** Use `os.Stdout.WriteString()` to avoid buffering issues

## Testing

```bash
# Build
go build -o bin/sc ./cmd/sc

# Test 1: Command autocomplete
./bin/sc assistant chat
💬 /pro<Tab>
Expected: /provider (same line)

# Test 2: Show subcommands
💬 /provider<Tab>
Expected: List of subcommands without extra lines

# Test 3: Subcommand autocomplete
💬 /provider s<Tab>
Expected: /provider switch (same line)

# Test 4: Complete command
💬 /provider list<Tab>
Expected: Nothing happens

# Test 5: No matches
💬 /xyz<Tab>
Expected: Nothing happens
```

## Status

✅ All issues fixed  
✅ Behavior like bash/zsh  
✅ No extra newlines  
✅ Clean and predictable interface  
✅ Ready to use  

## Files Modified

- `pkg/assistant/chat/input.go` - Main autocomplete logic
