package sources

import (
	"context"
	"errors"
	"testing"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubClient struct {
	plan *types.SourcePlan
	err  error
	lastSystemPrompt string
	lastUserPrompt   string
}

func (s *stubClient) Discover(ctx context.Context, systemPrompt, userPrompt string) (*types.SourcePlan, error) {
	s.lastSystemPrompt = systemPrompt
	s.lastUserPrompt = userPrompt
	return s.plan, s.err
}

func TestDiscover_ReturnsTrimmedPlan(t *testing.T) {
	plan := &types.SourcePlan{}
	for i := 0; i < 15; i++ {
		plan.Reddit.Subreddits = append(plan.Reddit.Subreddits, "sub"+string(rune('a'+i%26)))
	}
	c := &stubClient{plan: plan}
	a := &Agent{Claude: c}

	got, err := a.Discover(context.Background(), "soc2", "")
	require.NoError(t, err)
	assert.Len(t, got.Reddit.Subreddits, MaxSubreddits)
	assert.Contains(t, c.lastUserPrompt, "soc2")
}

func TestDiscover_PropagatesClientError(t *testing.T) {
	c := &stubClient{err: errors.New("boom")}
	a := &Agent{Claude: c}
	_, err := a.Discover(context.Background(), "t", "")
	require.Error(t, err)
}

func TestDiscover_RejectsEmptyPlan(t *testing.T) {
	c := &stubClient{plan: &types.SourcePlan{}}
	a := &Agent{Claude: c}
	_, err := a.Discover(context.Background(), "t", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestRediscover_IncludesCurrentStatsInPrompt(t *testing.T) {
	plan := &types.SourcePlan{}
	plan.Reddit.Subreddits = []string{"one"}
	c := &stubClient{plan: plan}
	a := &Agent{Claude: c}

	stats := []SourceStat{
		{Platform: types.PlatformReddit, Kind: types.SourceKindSubreddit, Value: "old", DocsLast7d: 3, AvgScore: 10, SignalScore: 0.3},
	}
	_, err := a.Rediscover(context.Background(), "t", "desc", stats)
	require.NoError(t, err)
	assert.Contains(t, c.lastUserPrompt, "old")
	assert.Contains(t, c.lastUserPrompt, "docs=3")
}
