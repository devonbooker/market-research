package main

import (
	"context"
	"time"

	rfetch "github.com/devonbooker/market-research/internal/fetch/reddit"
	sofetch "github.com/devonbooker/market-research/internal/fetch/stackoverflow"
	"github.com/devonbooker/market-research/internal/types"
)

type redditAdapter struct {
	client             *rfetch.Client
	topCommentsPerPost int
}

func (a *redditAdapter) FetchDocuments(ctx context.Context, src *types.Source, since time.Time, max int) ([]*types.Document, error) {
	switch src.Kind {
	case types.SourceKindSubreddit:
		return rfetch.FetchSubredditNew(ctx, a.client, src.Value, since, max)
	case types.SourceKindSearchQuery:
		return rfetch.FetchSearch(ctx, a.client, src.Value, "", since, max)
	}
	return nil, nil
}

func (a *redditAdapter) FetchReplies(ctx context.Context, platformID string) ([]*types.Reply, error) {
	n := a.topCommentsPerPost
	if n == 0 {
		n = 10
	}
	return rfetch.FetchTopComments(ctx, a.client, platformID, n)
}

type stackOverflowAdapter struct {
	client *sofetch.Client
}

func (a *stackOverflowAdapter) FetchDocuments(ctx context.Context, src *types.Source, since time.Time, max int) ([]*types.Document, error) {
	switch src.Kind {
	case types.SourceKindSOTag:
		return sofetch.FetchQuestionsByTag(ctx, a.client, src.Value, since, max)
	case types.SourceKindSearchQuery:
		return sofetch.FetchSearch(ctx, a.client, src.Value, since, max)
	}
	return nil, nil
}

func (a *stackOverflowAdapter) FetchReplies(ctx context.Context, platformID string) ([]*types.Reply, error) {
	// For SO we need the accepted_answer_id, which lives in the document's platform_metadata.
	// The orchestrator fetches by post platform_id (question_id). We load the document to find the accepted answer id.
	// Here we just return nil; in practice, FetchReplies for SO is wired via a closure in main.go that has store access.
	return nil, nil
}
