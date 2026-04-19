package sources

import (
	"fmt"
	"strings"

	"github.com/devonbooker/market-research/internal/types"
)

const systemPrompt = `You are a market research assistant that identifies the best sources to monitor on Reddit and Stack Overflow for a given topic.

Your only job is to call the submit_source_plan tool. Do not chat.

Constraints:
- Subreddit names: no "r/" prefix, no spaces, lowercase.
- Stack Overflow tags: lowercase, hyphenated where applicable (e.g., "kubernetes-helm").
- Search queries: full-text, quoted phrases OK. Prefer specific pain-language queries ("X is broken", "how to X", "alternatives to X").
- Return at most 10 subreddits, 5 SO tags, 5 search queries per platform.
- Prefer sources where end-users complain or ask questions about the topic. Avoid vendor-marketing subreddits.`

// InitialPrompt builds the user message for a first-time topic discovery.
func InitialPrompt(topicName, description string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Topic: %s\n", topicName)
	if description != "" {
		fmt.Fprintf(&b, "Description: %s\n", description)
	}
	fmt.Fprintln(&b, "\nThis is a new topic with no existing source list. Propose an initial plan.")
	return b.String()
}

// SourceStat is per-source data fed to the rediscovery prompt.
type SourceStat struct {
	Platform    types.Platform
	Kind        types.SourceKind
	Value       string
	DocsLast7d  int
	AvgScore    float64
	SignalScore float64
}

// RediscoverPrompt builds the user message for weekly source rediscovery.
func RediscoverPrompt(topicName, description string, current []SourceStat) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Topic: %s\n", topicName)
	if description != "" {
		fmt.Fprintf(&b, "Description: %s\n", description)
	}
	fmt.Fprintln(&b, "\nCurrent sources with performance over the last 7 days:")
	if len(current) == 0 {
		fmt.Fprintln(&b, "  (none)")
	} else {
		for _, s := range current {
			fmt.Fprintf(&b, "  - %s/%s: %s (docs=%d, avg_score=%.2f, signal=%.2f)\n",
				s.Platform, s.Kind, s.Value, s.DocsLast7d, s.AvgScore, s.SignalScore)
		}
	}
	fmt.Fprintln(&b, "\nExpand, prune, or reprioritize. Return a full new plan (not a diff).")
	return b.String()
}

// SystemPrompt returns the system prompt used for both initial discovery and rediscovery.
func SystemPrompt() string {
	return systemPrompt
}
