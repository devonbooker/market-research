package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/config"
	"github.com/devonbooker/market-research/internal/fetch"
	rfetch "github.com/devonbooker/market-research/internal/fetch/reddit"
	sofetch "github.com/devonbooker/market-research/internal/fetch/stackoverflow"
	"github.com/devonbooker/market-research/internal/sources"
	"github.com/devonbooker/market-research/internal/store"
	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubClaude struct{}

func (stubClaude) Discover(ctx context.Context, sys, user string) (*types.SourcePlan, error) {
	p := &types.SourcePlan{}
	p.Reddit.Subreddits = []string{"devsecops"}
	p.StackOverflow.Tags = []string{"soc2"}
	p.Reasoning = "test"
	return p, nil
}

func TestE2E_AddTopicThenFetchLandsDocsInDB(t *testing.T) {
	// --- Stub platform servers ---
	redditSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/access_token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case r.URL.Path == "/r/devsecops/new":
			_, _ = w.Write([]byte(`{"data":{"children":[{"data":{"id":"p1","subreddit":"devsecops","title":"soc2 pain","selftext":"body","author":"a","score":1,"permalink":"/r/devsecops/comments/p1/","url":"https://reddit.com/x","created_utc":` + itoa(time.Now().Unix()) + `}}]}}`))
		case r.URL.Path == "/comments/p1.json":
			_, _ = w.Write([]byte(`[{"data":{"children":[]}},{"data":{"children":[{"kind":"t1","data":{"id":"c1","body":"me too","author":"b","score":5,"created_utc":` + itoa(time.Now().Unix()) + `}}]}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer redditSrv.Close()

	soSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"question_id":1,"title":"q","body":"b","owner":{"display_name":"u"},"score":2,"tags":["soc2"],"link":"https://stackoverflow.com/questions/1","creation_date":` + itoa(time.Now().Unix()) + `,"is_answered":false}]}`))
	}))
	defer soSrv.Close()

	// --- Temp DB and config ---
	dbPath := filepath.Join(t.TempDir(), "e2e.db")
	cfg := &config.Config{
		RedditClientID:     "id",
		RedditClientSecret: "secret",
		RedditUserAgent:    "e2e",
		StackExchangeKey:   "k",
		AnthropicAPIKey:    "a",
		DBPath:             dbPath,
	}
	st, err := store.Open(cfg.DBPath)
	require.NoError(t, err)
	defer st.Close()

	// --- Create topic + apply stubbed source plan directly (skip real agent anthropic call) ---
	topicID, err := st.CreateTopic("soc2 compliance tool", "", false)
	require.NoError(t, err)
	plan, err := (&sources.Agent{Claude: stubClaude{}}).Discover(context.Background(), "soc2 compliance tool", "")
	require.NoError(t, err)
	for _, s := range sources.PlanToSources(plan) {
		_, _, err := st.UpsertSource(topicID, s.Platform, s.Kind, s.Value, types.AddedByAgent)
		require.NoError(t, err)
	}
	require.NoError(t, st.SetTopicActive(topicID, true))

	// --- Wire orchestrator pointed at test servers ---
	rc := rfetch.New(rfetch.Config{
		ClientID: "id", ClientSecret: "secret", UserAgent: "e2e",
		AuthURL: redditSrv.URL + "/api/v1/access_token", APIBaseURL: redditSrv.URL,
		RateLimit: 100,
	})
	soc := sofetch.New(sofetch.Config{Key: "k", APIBaseURL: soSrv.URL, RateLimit: 100})

	orch := &fetch.Orchestrator{
		Store:              st,
		Reddit:             &redditAdapter{client: rc, topCommentsPerPost: 10},
		StackOverflow:      &storeAwareSOAdapter{client: soc, store: st},
		BackfillWindow:     24 * time.Hour,
		MaxPostsPerSource:  100,
		TopCommentsPerPost: 10,
		Retries:            0,
	}
	require.NoError(t, orch.RunAll(context.Background()))

	// --- Assert results ---
	var docCount int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM documents").Scan(&docCount))
	assert.GreaterOrEqual(t, docCount, 2, "expected at least 1 reddit + 1 so doc")

	var replyCount int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM document_replies").Scan(&replyCount))
	assert.GreaterOrEqual(t, replyCount, 1, "expected the reddit comment to be stored")

	var successRuns int
	require.NoError(t, st.DB().QueryRow("SELECT COUNT(*) FROM fetch_runs WHERE status='success'").Scan(&successRuns))
	assert.Equal(t, 2, successRuns)
}

func itoa(i int64) string {
	// small helper to keep the template string above readable
	const chars = "0123456789"
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = chars[i%10]
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
