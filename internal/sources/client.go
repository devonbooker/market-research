package sources

import (
	"context"

	"github.com/devonbooker/market-research/internal/types"
)

// ClaudeClient is the narrow interface the source-discovery agent needs from Claude.
// Production uses an adapter over the anthropic-sdk-go; tests inject a stub.
type ClaudeClient interface {
	// Discover returns a SourcePlan from the given prompt. Implementations MUST
	// return an error (never a partial plan) if the model output cannot be coerced
	// into a valid SourcePlan via the forced tool call.
	Discover(ctx context.Context, systemPrompt, userPrompt string) (*types.SourcePlan, error)
}
