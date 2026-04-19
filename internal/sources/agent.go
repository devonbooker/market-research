package sources

import (
	"context"
	"fmt"

	"github.com/devonbooker/market-research/internal/types"
)

type Agent struct {
	Claude ClaudeClient
}

func (a *Agent) Discover(ctx context.Context, topicName, description string) (*types.SourcePlan, error) {
	plan, err := a.Claude.Discover(ctx, SystemPrompt(), InitialPrompt(topicName, description))
	if err != nil {
		return nil, fmt.Errorf("agent.Discover: %w", err)
	}
	if err := Validate(plan); err != nil {
		return nil, err
	}
	TrimToCaps(plan)
	return plan, nil
}

func (a *Agent) Rediscover(ctx context.Context, topicName, description string, current []SourceStat) (*types.SourcePlan, error) {
	plan, err := a.Claude.Discover(ctx, SystemPrompt(), RediscoverPrompt(topicName, description, current))
	if err != nil {
		return nil, fmt.Errorf("agent.Rediscover: %w", err)
	}
	if err := Validate(plan); err != nil {
		return nil, err
	}
	TrimToCaps(plan)
	return plan, nil
}
