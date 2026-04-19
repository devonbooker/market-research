package sources

import (
	"errors"
	"strings"

	"github.com/devonbooker/market-research/internal/types"
)

const (
	MaxSubreddits    = 10
	MaxSOTags        = 5
	MaxSearchQueries = 5
)

func Validate(p *types.SourcePlan) error {
	total := len(p.Reddit.Subreddits) + len(p.Reddit.SearchQueries) +
		len(p.StackOverflow.Tags) + len(p.StackOverflow.SearchQueries)
	if total == 0 {
		return errors.New("source plan is empty (no subreddits, tags, or queries)")
	}
	return nil
}

func TrimToCaps(p *types.SourcePlan) {
	p.Reddit.Subreddits = normalizeAndCap(p.Reddit.Subreddits, MaxSubreddits)
	p.Reddit.SearchQueries = normalizeAndCap(p.Reddit.SearchQueries, MaxSearchQueries)
	p.StackOverflow.Tags = normalizeAndCap(p.StackOverflow.Tags, MaxSOTags)
	p.StackOverflow.SearchQueries = normalizeAndCap(p.StackOverflow.SearchQueries, MaxSearchQueries)
}

func normalizeAndCap(items []string, cap int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, it := range items {
		n := strings.ToLower(strings.TrimSpace(it))
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
		if len(out) >= cap {
			break
		}
	}
	return out
}

// PlanToSources converts a validated + trimmed SourcePlan into (platform, kind, value) triples.
func PlanToSources(p *types.SourcePlan) []struct {
	Platform types.Platform
	Kind     types.SourceKind
	Value    string
} {
	var out []struct {
		Platform types.Platform
		Kind     types.SourceKind
		Value    string
	}
	for _, v := range p.Reddit.Subreddits {
		out = append(out, struct {
			Platform types.Platform
			Kind     types.SourceKind
			Value    string
		}{types.PlatformReddit, types.SourceKindSubreddit, v})
	}
	for _, v := range p.Reddit.SearchQueries {
		out = append(out, struct {
			Platform types.Platform
			Kind     types.SourceKind
			Value    string
		}{types.PlatformReddit, types.SourceKindSearchQuery, v})
	}
	for _, v := range p.StackOverflow.Tags {
		out = append(out, struct {
			Platform types.Platform
			Kind     types.SourceKind
			Value    string
		}{types.PlatformStackOverflow, types.SourceKindSOTag, v})
	}
	for _, v := range p.StackOverflow.SearchQueries {
		out = append(out, struct {
			Platform types.Platform
			Kind     types.SourceKind
			Value    string
		}{types.PlatformStackOverflow, types.SourceKindSearchQuery, v})
	}
	return out
}
