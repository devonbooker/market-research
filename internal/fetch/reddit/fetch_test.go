package reddit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveFixture(t *testing.T, path string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/access_token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		default:
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
		}
	}))
}

func TestFetchSubredditNew_FiltersBySince(t *testing.T) {
	srv := serveFixture(t, "testdata/subreddit_new.json")
	defer srv.Close()

	c := New(Config{
		ClientID: "c", ClientSecret: "s", UserAgent: "ua",
		AuthURL:    srv.URL + "/api/v1/access_token",
		APIBaseURL: srv.URL,
		RateLimit:  100,
	})

	since := time.Unix(1710000000, 0).UTC()
	docs, err := FetchSubredditNew(context.Background(), c, "devsecops", since, 100)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	d := docs[0]
	assert.Equal(t, types.PlatformReddit, d.Platform)
	assert.Equal(t, "p1", d.PlatformID)
	assert.Equal(t, "SOC2 evidence collection is painful", d.Title)
	assert.Equal(t, "We spend hours on this every quarter.", d.Body)
	assert.Equal(t, "alice", d.Author)
	assert.Equal(t, 42, d.Score)
	assert.Contains(t, d.URL, "/r/devsecops/comments/p1/")
	assert.True(t, d.CreatedAt.Equal(time.Unix(1713000000, 0).UTC()))
	assert.Contains(t, string(d.PlatformMetadata), "devsecops")
	assert.Contains(t, string(d.PlatformMetadata), "Question")
}

func TestFetchSubredditNew_CapsAtLimit(t *testing.T) {
	srv := serveFixture(t, "testdata/subreddit_new.json")
	defer srv.Close()

	c := New(Config{
		ClientID: "c", ClientSecret: "s", UserAgent: "ua",
		AuthURL: srv.URL + "/api/v1/access_token", APIBaseURL: srv.URL,
		RateLimit: 100,
	})

	docs, err := FetchSubredditNew(context.Background(), c, "devsecops", time.Time{}, 1)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}

func TestFetchTopComments_ReturnsTopNSkippingMoreKind(t *testing.T) {
	srv := serveFixture(t, "testdata/comments.json")
	defer srv.Close()

	c := New(Config{
		ClientID: "c", ClientSecret: "s", UserAgent: "ua",
		AuthURL: srv.URL + "/api/v1/access_token", APIBaseURL: srv.URL,
		RateLimit: 100,
	})

	replies, err := FetchTopComments(context.Background(), c, "p1", 10)
	require.NoError(t, err)
	require.Len(t, replies, 2)
	assert.Equal(t, "c1", replies[0].PlatformID)
	assert.Equal(t, "Same problem here, tried X and Y and nothing works.", replies[0].Body)
	assert.Equal(t, "alice", replies[0].Author)
	assert.Equal(t, 25, replies[0].Score)
}
