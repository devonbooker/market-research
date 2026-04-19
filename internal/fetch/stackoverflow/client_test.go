package stackoverflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetJSON_SetsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "mykey", r.URL.Query().Get("key"))
		assert.Equal(t, "stackoverflow", r.URL.Query().Get("site"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	c := New(Config{Key: "mykey", APIBaseURL: srv.URL, RateLimit: 100})
	var resp struct{ Items []any }
	require.NoError(t, c.GetJSON(context.Background(), "/questions", nil, &resp))
}

func TestClient_GetJSON_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()
	c := New(Config{Key: "k", APIBaseURL: srv.URL, RateLimit: 100})
	err := c.GetJSON(context.Background(), "/whatever", nil, &struct{}{})
	require.Error(t, err)
	var he *HTTPError
	assert.ErrorAs(t, err, &he)
	assert.Equal(t, 400, he.Status)
}
