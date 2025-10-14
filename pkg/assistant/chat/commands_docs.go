package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/simple-container-com/api/pkg/assistant/embeddings"
)

// registerDocsCommands registers documentation search commands
func (c *ChatInterface) registerDocsCommands() {
	c.commands["search_docs"] = &ChatCommand{
		Name:        "search_docs",
		Description: "Search Simple Container documentation for specific information",
		Usage:       "/search_docs <query>",
		Handler:     c.handleSearchDocs,
		Args: []CommandArg{
			{Name: "query", Type: "string", Required: true, Description: "Search query for documentation"},
		},
	}
}

// handleSearchDocs searches Simple Container documentation
func (c *ChatInterface) handleSearchDocs(ctx context.Context, args []string, context *ConversationContext) (*CommandResult, error) {
	if len(args) == 0 {
		return &CommandResult{
			Success: false,
			Message: "❌ Usage: /search_docs <query>\nExample: /search_docs mongodb configuration",
		}, nil
	}

	query := strings.Join(args, " ")
	limit := 5 // Default limit for docs search

	if c.embeddings == nil {
		return &CommandResult{
			Success: false,
			Message: "❌ Documentation search is not available - embeddings database not loaded",
		}, nil
	}

	results, err := embeddings.SearchDocumentation(c.embeddings, query, limit)
	if err != nil {
		return &CommandResult{
			Success: false,
			Message: fmt.Sprintf("❌ Documentation search failed: %v", err),
		}, nil
	}

	if len(results) == 0 {
		return &CommandResult{
			Success: true,
			Message: fmt.Sprintf("🔍 No documentation results found for: \"%s\"\n\n💡 Try:\n- More general terms\n- Different keywords\n- '/search <query>' for broader search", query),
		}, nil
	}

	message := fmt.Sprintf("📚 Documentation Results for \"%s\":\n\n", query)

	for i, result := range results {
		score := int(result.Score * 100)

		// Extract metadata
		title := "Unknown Document"
		if titleInterface, exists := result.Metadata["title"]; exists {
			if titleStr, ok := titleInterface.(string); ok {
				title = titleStr
			}
		}

		path := "Unknown Path"
		if pathInterface, exists := result.Metadata["path"]; exists {
			if pathStr, ok := pathInterface.(string); ok {
				path = pathStr
			}
		}

		docType := "docs"
		if blockIndex, exists := result.Metadata["block_index"]; exists {
			if _, ok := blockIndex.(int); ok {
				docType = "code block"
			}
		}

		// Show document with score and type
		message += fmt.Sprintf("**%d. %s** (%d%% match - %s)\n", i+1, title, score, docType)
		message += fmt.Sprintf("   📁 Path: `%s`\n", path)

		// Show relevant content snippet
		content := result.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}

		// Clean up content for better display
		content = strings.ReplaceAll(content, "\n\n", " ")
		content = strings.ReplaceAll(content, "\n", " ")
		content = strings.TrimSpace(content)

		message += fmt.Sprintf("   📝 %s\n\n", content)
	}

	message += "💡 **Tips:**\n"
	message += "- Use more specific terms for better results\n"
	message += "- Try '/search <query>' for broader search including examples\n"
	message += "- Check the full documentation at the provided paths"

	return &CommandResult{
		Success: true,
		Message: message,
	}, nil
}
