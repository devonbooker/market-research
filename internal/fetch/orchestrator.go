package fetch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/devonbooker/market-research/internal/store"
	"github.com/devonbooker/market-research/internal/types"
)

// PlatformFetcher is the boundary between the orchestrator and concrete per-platform code.
// Reddit and StackOverflow packages implement this via small adapters (see cmd/mr/main.go wiring).
type PlatformFetcher interface {
	FetchDocuments(ctx context.Context, src *types.Source, since time.Time, maxPosts int) ([]*types.Document, error)
	FetchReplies(ctx context.Context, docPlatformID string) ([]*types.Reply, error)
}

type Orchestrator struct {
	Store         *store.Store
	Reddit        PlatformFetcher
	StackOverflow PlatformFetcher

	// Tunables
	BackfillWindow     time.Duration // window for first-time fetch on a new source
	MaxPostsPerSource  int           // cap per source per run
	TopCommentsPerPost int           // comment cap per post
	Retries            int           // transient retry count (default 3)
	BackoffBase        time.Duration // base for exponential backoff (default 500ms)
}

func (o *Orchestrator) RunAll(ctx context.Context) error {
	topics, err := o.Store.ListTopics(false)
	if err != nil {
		return err
	}
	for _, t := range topics {
		o.runTopic(ctx, t)
	}
	return nil
}

func (o *Orchestrator) RunTopic(ctx context.Context, name string) error {
	t, err := o.Store.GetTopicByName(name)
	if err != nil {
		return err
	}
	o.runTopic(ctx, t)
	return nil
}

func (o *Orchestrator) runTopic(ctx context.Context, t *types.Topic) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("topic panic", "topic", t.Name, "panic", r)
		}
	}()

	var wg sync.WaitGroup
	for _, pair := range []struct {
		platform types.Platform
		fetcher  PlatformFetcher
	}{
		{types.PlatformReddit, o.Reddit},
		{types.PlatformStackOverflow, o.StackOverflow},
	} {
		wg.Add(1)
		go func(platform types.Platform, fetcher PlatformFetcher) {
			defer wg.Done()
			o.runPlatform(ctx, t, platform, fetcher)
		}(pair.platform, pair.fetcher)
	}
	wg.Wait()
}

func (o *Orchestrator) runPlatform(ctx context.Context, t *types.Topic, platform types.Platform, fetcher PlatformFetcher) {
	runID, err := o.Store.StartFetchRun(t.ID, platform)
	if err != nil {
		slog.Error("start fetch run", "err", err)
		return
	}

	var docsNew, repliesNew int
	var lastErr error

	sources, err := o.Store.ListSources(t.ID, platform, false)
	if err != nil {
		_ = o.Store.CloseFetchRun(runID, types.RunStatusError, 0, 0, err.Error())
		return
	}

	for _, src := range sources {
		since := o.backfillStart(src)
		docs, err := o.fetchWithRetry(ctx, fetcher, src, since)
		if err != nil {
			lastErr = err
			slog.Error("fetch source failed", "topic", t.Name, "source", src.Value, "err", err)
			if IsPermanent(err) {
				_ = o.Store.SetSourceActive(src.ID, false)
			}
			continue
		}

		for _, d := range docs {
			d.TopicID = t.ID
			d.SourceID = src.ID
			id, inserted, err := o.Store.UpsertDocument(d)
			if err != nil {
				lastErr = err
				continue
			}
			if inserted {
				docsNew++
			}
			replies, err := fetcher.FetchReplies(ctx, d.PlatformID)
			if err != nil {
				slog.Warn("fetch replies", "platform_id", d.PlatformID, "err", err)
				continue
			}
			for _, r := range replies {
				r.DocumentID = id
				if _, ins, err := o.Store.UpsertReply(r); err == nil && ins {
					repliesNew++
				}
			}
		}
		_ = o.Store.UpdateSourceLastFetched(src.ID, time.Now().UTC())
	}

	status := types.RunStatusSuccess
	errMsg := ""
	if lastErr != nil {
		status = types.RunStatusError
		errMsg = lastErr.Error()
	}
	_ = o.Store.CloseFetchRun(runID, status, docsNew, repliesNew, errMsg)
}

func (o *Orchestrator) backfillStart(src *types.Source) time.Time {
	if src.LastFetched != nil {
		return *src.LastFetched
	}
	w := o.BackfillWindow
	if w == 0 {
		w = 7 * 24 * time.Hour
	}
	return time.Now().Add(-w).UTC()
}

func (o *Orchestrator) fetchWithRetry(ctx context.Context, fetcher PlatformFetcher, src *types.Source, since time.Time) ([]*types.Document, error) {
	retries := o.Retries
	if retries == 0 {
		retries = 3
	}
	base := o.BackoffBase
	if base == 0 {
		base = 500 * time.Millisecond
	}
	maxPosts := o.MaxPostsPerSource
	if maxPosts == 0 {
		maxPosts = 100
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		docs, err := fetcher.FetchDocuments(ctx, src, since, maxPosts)
		if err == nil {
			return docs, nil
		}
		lastErr = err
		if IsPermanent(err) {
			return nil, err
		}
		if !IsTransient(err) {
			return nil, err
		}
		if attempt == retries {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(base * (1 << attempt)):
		}
	}
	return nil, fmt.Errorf("exhausted retries: %w", lastErr)
}
