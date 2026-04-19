package sources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/devonbooker/market-research/internal/types"
)

type AnthropicClient struct {
	client *anthropic.Client
	model  string
}

func NewAnthropicClient(apiKey string) *AnthropicClient {
	c := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &AnthropicClient{client: &c, model: anthropic.ModelClaudeSonnet4_6}
}

func (a *AnthropicClient) Discover(ctx context.Context, systemPrompt, userPrompt string) (*types.SourcePlan, error) {
	tool := anthropic.ToolParam{
		Name:        "submit_source_plan",
		Description: anthropic.String("Submit the source plan for the given topic."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"reddit": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"subreddits":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"search_queries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
					"required": []string{"subreddits", "search_queries"},
				},
				"stackoverflow": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tags":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"search_queries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
					"required": []string{"tags", "search_queries"},
				},
				"reasoning": map[string]any{"type": "string"},
			},
			Required: []string{"reddit", "stackoverflow", "reasoning"},
		},
	}

	msg, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: "submit_source_plan"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic messages.new: %w", err)
	}

	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			tu := block.AsToolUse()
			if tu.Name == "submit_source_plan" {
				var plan types.SourcePlan
				if err := json.Unmarshal(tu.Input, &plan); err != nil {
					return nil, fmt.Errorf("unmarshal tool input: %w", err)
				}
				return &plan, nil
			}
		}
	}
	return nil, fmt.Errorf("model did not call submit_source_plan tool")
}
