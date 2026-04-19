package main

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/devonbooker/market-research/internal/fetch"
	rfetch "github.com/devonbooker/market-research/internal/fetch/reddit"
	sofetch "github.com/devonbooker/market-research/internal/fetch/stackoverflow"
	"github.com/devonbooker/market-research/internal/store"
	"github.com/devonbooker/market-research/internal/types"
	"github.com/spf13/cobra"
)

func newFetchCmd(rt *runtime) *cobra.Command {
	var all bool
	var topic string
	c := &cobra.Command{
		Use:   "fetch",
		Short: "fetch new content for topics (daily job)",
		RunE: func(cmd *cobra.Command, args []string) error {
			orch := buildOrchestrator(rt)
			if all || topic == "" {
				return orch.RunAll(cmd.Context())
			}
			return orch.RunTopic(cmd.Context(), topic)
		},
	}
	c.Flags().BoolVar(&all, "all", false, "fetch all active topics (default)")
	c.Flags().StringVar(&topic, "topic", "", "fetch a single topic by name")
	return c
}

func buildOrchestrator(rt *runtime) *fetch.Orchestrator {
	rc := rfetch.New(rfetch.Config{
		ClientID:     rt.cfg.RedditClientID,
		ClientSecret: rt.cfg.RedditClientSecret,
		UserAgent:    rt.cfg.RedditUserAgent,
	})
	soc := sofetch.New(sofetch.Config{Key: rt.cfg.StackExchangeKey})

	return &fetch.Orchestrator{
		Store:              rt.store,
		Reddit:             &redditAdapter{client: rc, topCommentsPerPost: 10},
		StackOverflow:      &storeAwareSOAdapter{client: soc, store: rt.store},
		BackfillWindow:     7 * 24 * time.Hour,
		MaxPostsPerSource:  100,
		TopCommentsPerPost: 10,
		Retries:            3,
		BackoffBase:        500 * time.Millisecond,
	}
}

// storeAwareSOAdapter pulls accepted_answer_id from the stored document metadata
// and fetches it as the reply.
type storeAwareSOAdapter struct {
	client *sofetch.Client
	store  *store.Store
}

func (a *storeAwareSOAdapter) FetchDocuments(ctx context.Context, src *types.Source, since time.Time, max int) ([]*types.Document, error) {
	switch src.Kind {
	case types.SourceKindSOTag:
		return sofetch.FetchQuestionsByTag(ctx, a.client, src.Value, since, max)
	case types.SourceKindSearchQuery:
		return sofetch.FetchSearch(ctx, a.client, src.Value, since, max)
	}
	return nil, nil
}

func (a *storeAwareSOAdapter) FetchReplies(ctx context.Context, platformID string) ([]*types.Reply, error) {
	// Look up the document by (platform, platform_id) to find accepted_answer_id in metadata.
	var metaRaw []byte
	err := a.store.DB().QueryRow(
		`SELECT platform_metadata FROM documents WHERE platform = ? AND platform_id = ?`,
		types.PlatformStackOverflow, platformID,
	).Scan(&metaRaw)
	if err != nil {
		return nil, err
	}
	var meta struct {
		AcceptedAnswerID *int64 `json:"accepted_answer_id"`
	}
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return nil, err
	}
	if meta.AcceptedAnswerID == nil {
		return nil, nil
	}
	reply, err := sofetch.FetchAcceptedAnswer(ctx, a.client, *meta.AcceptedAnswerID)
	if err != nil || reply == nil {
		return nil, err
	}
	return []*types.Reply{reply}, nil
}

var _ = strconv.Itoa
