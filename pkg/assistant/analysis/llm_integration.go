package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/simple-container-com/api/pkg/assistant/embeddings"
)

// enhanceWithLLM uses LLM to provide deeper insights and better recommendations
func (pa *ProjectAnalyzer) enhanceWithLLM(ctx context.Context, analysis *ProjectAnalysis) (*ProjectAnalysis, error) {
	if pa.llmProvider == nil {
		return analysis, nil
	}

	// Prepare project context for LLM analysis
	projectSummary := pa.buildProjectSummary(analysis)

	// Get relevant documentation context (limit to reduce tokens)
	var docContext string
	if pa.embeddingsDB != nil {
		if results, err := embeddings.SearchDocumentation(pa.embeddingsDB,
			fmt.Sprintf("%s %s architecture patterns", analysis.PrimaryStack.Language, analysis.PrimaryStack.Framework), 2); err == nil {
			for _, result := range results {
				// Truncate each result to max 200 characters to control token usage
				content := result.Content
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				docContext += content + "\n"
			}
		}
	}

	// Build LLM prompt for enhanced analysis
	prompt := pa.buildAnalysisPrompt(projectSummary, docContext)

	// Check token usage limits before making expensive LLM call
	estimatedTokens := len(prompt) / 4 // Rough approximation
	if pa.maxTokens > 0 && estimatedTokens > pa.maxTokens && pa.skipLLMIfExpensive {
		pa.progressReporter.ReportProgress("llm_skipped", fmt.Sprintf("Skipping LLM enhancement (estimated %d tokens > limit %d)", estimatedTokens, pa.maxTokens), 95)
		return analysis, nil // Skip LLM to save costs
	}

	pa.progressReporter.ReportProgress("llm_processing", fmt.Sprintf("Sending %d estimated tokens to LLM...", estimatedTokens), 92)

	// Get LLM insights
	response, err := pa.llmProvider.GenerateResponse(ctx, prompt)
	if err != nil {
		return analysis, err // Return original analysis if LLM fails
	}

	// Parse LLM response and enhance analysis
	enhanced := pa.parseAndEnhanceAnalysis(analysis, response)

	return enhanced, nil
}

// buildProjectSummary creates a concise summary for LLM analysis
func (pa *ProjectAnalyzer) buildProjectSummary(analysis *ProjectAnalysis) string {
	var summary strings.Builder

	summary.WriteString(fmt.Sprintf("Project: %s\n", analysis.Name))
	summary.WriteString(fmt.Sprintf("Architecture: %s\n", analysis.Architecture))

	if analysis.PrimaryStack != nil {
		summary.WriteString(fmt.Sprintf("Primary Technology: %s %s (%.1f%% confidence)\n",
			analysis.PrimaryStack.Language,
			analysis.PrimaryStack.Framework,
			analysis.PrimaryStack.Confidence*100))
	}

	if len(analysis.TechStacks) > 1 {
		summary.WriteString("Additional Technologies:\n")
		for i, stack := range analysis.TechStacks[1:] {
			if i >= 3 { // Limit to prevent token explosion
				summary.WriteString("...\n")
				break
			}
			summary.WriteString(fmt.Sprintf("- %s %s (%.1f%%)\n",
				stack.Language, stack.Framework, stack.Confidence*100))
		}
	}

	if analysis.Resources != nil {
		summary.WriteString(fmt.Sprintf("Resources: %d env vars, %d databases, %d APIs\n",
			len(analysis.Resources.EnvironmentVars),
			len(analysis.Resources.Databases),
			len(analysis.Resources.ExternalAPIs)))

		// Include key databases
		if len(analysis.Resources.Databases) > 0 {
			summary.WriteString("Databases: ")
			for i, db := range analysis.Resources.Databases {
				if i >= 3 {
					summary.WriteString("...")
					break
				}
				if i > 0 {
					summary.WriteString(", ")
				}
				summary.WriteString(db.Type)
			}
			summary.WriteString("\n")
		}

		// Include key APIs
		if len(analysis.Resources.ExternalAPIs) > 0 {
			summary.WriteString("APIs: ")
			for i, api := range analysis.Resources.ExternalAPIs {
				if i >= 3 {
					summary.WriteString("...")
					break
				}
				if i > 0 {
					summary.WriteString(", ")
				}
				summary.WriteString(api.Name)
			}
			summary.WriteString("\n")
		}
	}

	if analysis.Git != nil && analysis.Git.IsGitRepo {
		summary.WriteString(fmt.Sprintf("Git: %s branch", analysis.Git.Branch))
		if analysis.Git.CommitActivity != nil {
			summary.WriteString(fmt.Sprintf(", %d commits", analysis.Git.CommitActivity.TotalCommits))
		}
		summary.WriteString("\n")
	}

	return summary.String()
}

// minFloat32 helper function for float32
func minFloat32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// buildAnalysisPrompt creates the LLM prompt for enhanced analysis
func (pa *ProjectAnalyzer) buildAnalysisPrompt(projectSummary, docContext string) string {
	prompt := `You are a senior DevOps engineer analyzing a software project for Simple Container deployment recommendations.

PROJECT ANALYSIS:
` + projectSummary + `

SIMPLE CONTAINER DOCUMENTATION CONTEXT:
` + docContext + `

Based on this project analysis, provide enhanced recommendations for Simple Container deployment in JSON format:

{
  "insights": ["key insight 1", "key insight 2", ...],
  "recommendations": [
    {
      "type": "deployment|resource|security|optimization",
      "priority": "high|medium|low", 
      "title": "Brief title",
      "description": "Detailed recommendation",
      "action": "specific action to take",
      "resource": "simple-container resource type if applicable"
    }
  ],
  "architecture_suggestions": ["suggestion 1", "suggestion 2", ...],
  "potential_issues": ["issue 1", "issue 2", ...]
}

Focus on:
1. Simple Container specific deployment patterns
2. Resource optimization opportunities  
3. Security improvements
4. Performance considerations
5. Scalability recommendations

Keep recommendations practical and actionable.`

	return prompt
}

// parseAndEnhanceAnalysis parses LLM response and enhances the analysis
func (pa *ProjectAnalyzer) parseAndEnhanceAnalysis(analysis *ProjectAnalysis, llmResponse string) *ProjectAnalysis {
	// Create enhanced copy
	enhanced := *analysis

	// Try to parse JSON response
	var enhancedData struct {
		Insights                []string         `json:"insights"`
		Recommendations         []Recommendation `json:"recommendations"`
		ArchitectureSuggestions []string         `json:"architecture_suggestions"`
		PotentialIssues         []string         `json:"potential_issues"`
	}

	if err := json.Unmarshal([]byte(llmResponse), &enhancedData); err != nil {
		// If JSON parsing fails, store the raw response in metadata
		enhanced.Metadata["llm_insights"] = llmResponse
		enhanced.Metadata["llm_enhanced"] = true
		return &enhanced
	}

	// Add LLM recommendations to existing ones
	enhanced.Recommendations = append(enhanced.Recommendations, enhancedData.Recommendations...)

	// Store additional insights in metadata
	enhanced.Metadata["llm_insights"] = enhancedData.Insights
	enhanced.Metadata["architecture_suggestions"] = enhancedData.ArchitectureSuggestions
	enhanced.Metadata["potential_issues"] = enhancedData.PotentialIssues
	enhanced.Metadata["llm_enhanced"] = true

	return &enhanced
}
