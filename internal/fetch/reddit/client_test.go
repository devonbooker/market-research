package reddit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetJSON_AuthorizesAndDecodes(t *testing.T) {
	var authCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/access_token":
			authCount++
			require.Equal(t, "POST", r.Method)
			username, password, ok := r.BasicAuth()
			require.True(t, ok)
			assert.Equal(t, "cid", username)
			assert.Equal(t, "csec", password)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer","expires_in":3600}`))
		case "/r/devsecops/new":
			assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
			assert.Contains(t, r.Header.Get("User-Agent"), "test-agent")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"children":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		UserAgent:    "test-agent",
		AuthURL:      srv.URL + "/api/v1/access_token",
		APIBaseURL:   srv.URL,
		RateLimit:    50.0,
	})

	var got struct{ Data struct{ Children []any } }
	require.NoError(t, c.GetJSON(context.Background(), "/r/devsecops/new", &got))
	assert.Equal(t, 1, authCount)

	require.NoError(t, c.GetJSON(context.Background(), "/r/devsecops/new", &got))
	assert.Equal(t, 1, authCount, "token should be reused while valid")
}

func TestClient_GetJSON_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "access_token") {
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer","expires_in":3600}`))
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	c := New(Config{
		ClientID:     "cid",
		ClientSecret: "csec",
		UserAgent:    "a",
		AuthURL:      srv.URL + "/api/v1/access_token",
		APIBaseURL:   srv.URL,
		RateLimit:    50.0,
	})

	err := c.GetJSON(context.Background(), "/anywhere", &struct{}{})
	require.Error(t, err)
	var he *HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, http.StatusForbidden, he.Status)
}

var _ = json.Marshal
var _ = time.Second
