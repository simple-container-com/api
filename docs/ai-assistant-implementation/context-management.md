# Context Management for LLM

## Overview

Implemented an intelligent context management system using **priority-based sliding window**. This ensures optimal usage of model context while preserving important information.

## Message Priorities

### 🔴 Critical Priority (always included)

1. **System prompt** - first system message, defines model behavior
2. **Last message** - current user request (LLM API requirement)

### 🟡 High Priority (included when space available)

3. **Messages with tool calls** - function/tool invocations
4. **Tool call results** - responses from tools (role: "tool")

### 🟢 Normal Priority (fills remaining space)

5. **Conversation history** - trimmed from oldest to newest (sliding window)

## Algorithm

### Step 1: Message Separation

```
Input messages → [System] + [History] + [Last]
```

- Takes **FIRST** system message only (bug fixed)
- All other messages are considered history
- Last message is extracted separately

### Step 2: Token Allocation

```
Available context = ModelContextSize - ReserveTokens

Allocation:
├─ System prompt: up to 30% of context (or full if smaller)
├─ Last message: fully (priority #1)
└─ History: remaining space
```

### Step 3: Reserve Critical Elements

**System prompt:**
- If ≤ 30% of context → include fully
- If > 30% and ≤ 50% → include fully (extreme case)
- If > 50% → truncate to 30% with marker `[System prompt truncated to fit context]`

**Last message:**
- Always included fully
- If doesn't fit with system prompt → reduce system prompt
- If still doesn't fit → remove system prompt

### Step 4: Fill History with Priorities

```python
available_tokens = context - system - last_message

# Phase 1: High priority messages
for msg in history (newest to oldest):
    if msg.has_tool_calls() or msg.role == "tool":
        if fits(msg, available_tokens):
            add(msg)
            available_tokens -= msg.tokens

# Phase 2: Normal priority messages
for msg in history (newest to oldest):
    if fits(msg, available_tokens):
        add(msg)
        available_tokens -= msg.tokens
```

### Step 5: Build Result

```
Result = [System prompt] + [History (chronologically sorted)] + [Last message]
```

## Usage Examples

### Example 1: Regular Conversation

```go
messages := []Message{
    {Role: "system", Content: "You are a helpful assistant"},
    {Role: "user", Content: "Hello"},
    {Role: "assistant", Content: "Hi! How can I help?"},
    {Role: "user", Content: "What's the weather?"},
}

trimmed := TrimMessagesToContextSize(messages, "gpt-4", 2048)
// Result: all messages (fit in context)
```

### Example 2: Long History

```go
messages := []Message{
    {Role: "system", Content: "System prompt"},
    // ... 50 history messages ...
    {Role: "user", Content: "Latest question"},
}

trimmed := TrimMessagesToContextSize(messages, "gpt-3.5-turbo", 2048)
// Result: system + last N messages + latest question
```

### Example 3: With Tool Calls

```go
messages := []Message{
    {Role: "system", Content: "System prompt"},
    {Role: "user", Content: "Old question 1"},
    {Role: "assistant", Content: "Old answer 1"},
    {Role: "user", Content: "Search for X"},
    {Role: "assistant", Content: "Searching...", Metadata: map[string]interface{}{
        "tool_calls": []interface{}{...},
    }},
    {Role: "tool", Content: "Search results"},
    {Role: "user", Content: "Old question 2"},
    {Role: "assistant", Content: "Old answer 2"},
    {Role: "user", Content: "Latest question"},
}

trimmed := TrimMessagesToContextSize(messages, "gpt-4", 2048)
// Result: system + tool call messages (priority) + latest question
// "Old question 1/2" may be removed, but tool calls preserved
```

## Safety Guarantees

### ✅ What is ALWAYS guaranteed:

1. **At least one non-system message** - LLM API requirement
2. **Last message included** - current request always processed
3. **Chronological order** - messages not reordered
4. **No limit exceeded** - model context always respected
5. **First system message** - takes first, not last

### ✅ Edge Case Handling:

| Case | Solution |
|------|----------|
| Huge system prompt | Truncate to 30% of context |
| Huge last message | Reduce/remove system prompt |
| Both huge | Remove system, keep last |
| History doesn't fit | Take what fits from end |
| Empty list | Return as is |

## Metrics and Debugging

### Trimming Logs

In `chat/interface.go` automatically logged:

```
📊 Trimmed 5 old messages to fit context window 
   (model: gpt-4, context: 8192 tokens, tools: 150 tokens)
```

### Result Verification

```go
original := len(messages)
trimmed := TrimMessagesToContextSize(messages, model, reserve)
dropped := original - len(trimmed)

fmt.Printf("Dropped %d messages, kept %d\n", dropped, len(trimmed))
```

## Testing

Tests added for all scenarios:

- ✅ `TestTrimMessagesToContextSize` - basic trimming
- ✅ `TestTrimMessagesToContextSize_PreservesSystemMessage` - system preservation
- ✅ `TestTrimMessagesToContextSize_PrioritizesToolCalls` - tool calls priority
- ✅ `TestTrimMessagesToContextSize_HandlesHugeLastMessage` - huge last message
- ✅ `TestTrimMessagesToContextSize_TakesFirstSystemMessage` - first system message

Run tests:

```bash
go test -v ./pkg/assistant/llm -run TestTrimMessagesToContextSize
```

## Performance

### Algorithm Complexity

- **Time**: O(n) where n is number of messages
- **Memory**: O(n) for storing result
- **Optimization**: One pass through history for categorization, one for selection

### Token Estimation

Uses fast approximation:

```go
tokens ≈ len(content) / 4 + 10 (overhead)
```

For accurate estimation, integrate `tiktoken` or similar.

## Future Improvements

### Possible Optimizations:

1. **Smart pair preservation** - keep user-assistant pairs together
2. **Semantic importance** - content analysis for prioritization
3. **Token caching** - save token counts
4. **Configurable priorities** - settings via Config
5. **Usage metrics** - trimming statistics for monitoring

### Tiktoken Integration:

```go
import "github.com/pkoukk/tiktoken-go"

func estimateMessageTokens(msg Message) int {
    encoding, _ := tiktoken.EncodingForModel("gpt-4")
    tokens := encoding.Encode(msg.Content, nil, nil)
    return len(tokens) + 10 // overhead
}
```

## Migration from Previous Version

### Behavior Changes:

1. **Bug fixed**: Now takes **first** system message, not last
2. **Priorities**: Messages with tool calls preserved with high priority
3. **Guarantees**: Last message **always** included, even at expense of system prompt

### Backward Compatibility:

✅ Function signature unchanged:

```go
func TrimMessagesToContextSize(messages []Message, model string, reserveTokens int) []Message
```

✅ All existing tests pass

✅ Behavior improved but compatible with previous version

## Conclusion

New implementation provides:

- ✅ **Efficient context usage** - maximum information within limits
- ✅ **Intelligent prioritization** - important messages preserved
- ✅ **Safety** - all edge cases handled
- ✅ **Predictability** - clear behavior in any situation
- ✅ **Testability** - full test coverage

System ready for production use.
