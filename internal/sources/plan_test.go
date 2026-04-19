package sources

import (
	"testing"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_RejectsEmpty(t *testing.T) {
	var p types.SourcePlan
	err := Validate(&p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestTrimToCaps_TrimsLongLists(t *testing.T) {
	p := types.SourcePlan{}
	for i := 0; i < 20; i++ {
		p.Reddit.Subreddits = append(p.Reddit.Subreddits, "s"+string(rune(i)))
	}
	for i := 0; i < 10; i++ {
		p.Reddit.SearchQueries = append(p.Reddit.SearchQueries, "q"+string(rune(i)))
	}
	for i := 0; i < 10; i++ {
		p.StackOverflow.Tags = append(p.StackOverflow.Tags, "t"+string(rune(i)))
	}
	for i := 0; i < 10; i++ {
		p.StackOverflow.SearchQueries = append(p.StackOverflow.SearchQueries, "q"+string(rune(i)))
	}

	TrimToCaps(&p)
	assert.Len(t, p.Reddit.Subreddits, MaxSubreddits)
	assert.Len(t, p.Reddit.SearchQueries, MaxSearchQueries)
	assert.Len(t, p.StackOverflow.Tags, MaxSOTags)
	assert.Len(t, p.StackOverflow.SearchQueries, MaxSearchQueries)
}

func TestTrimToCaps_NormalizesWhitespaceAndDedups(t *testing.T) {
	p := types.SourcePlan{}
	p.Reddit.Subreddits = []string{" DevSecOps ", "devsecops", "Cybersecurity"}
	TrimToCaps(&p)
	assert.Equal(t, []string{"devsecops", "cybersecurity"}, p.Reddit.Subreddits)
}
