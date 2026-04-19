package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/devonbooker/market-research/internal/types"
)

type listingResponse struct {
	Data struct {
		Children []struct {
			Data postData `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type postData struct {
	ID            string  `json:"id"`
	Subreddit     string  `json:"subreddit"`
	Title         string  `json:"title"`
	SelfText      string  `json:"selftext"`
	Author        string  `json:"author"`
	Score         int     `json:"score"`
	Permalink     string  `json:"permalink"`
	URL           string  `json:"url"`
	CreatedUTC    float64 `json:"created_utc"`
	LinkFlairText string  `json:"link_flair_text"`
}

const redditPermalinkBase = "https://reddit.com"

// FetchSubredditNew pulls /r/{name}/new, returns documents created after `since`, capped at maxPosts.
func FetchSubredditNew(ctx context.Context, c *Client, subreddit string, since time.Time, maxPosts int) ([]*types.Document, error) {
	path := fmt.Sprintf("/r/%s/new?limit=100", url.PathEscape(subreddit))
	var resp listingResponse
	if err := c.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	return toDocs(resp.Data.Children, since, maxPosts)
}

// FetchSearch runs a query scoped to a specific subreddit or sitewide if subreddit is empty.
func FetchSearch(ctx context.Context, c *Client, query, subreddit string, since time.Time, maxPosts int) ([]*types.Document, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("sort", "new")
	q.Set("limit", "100")
	path := "/search?" + q.Encode()
	if subreddit != "" {
		q.Set("restrict_sr", "on")
		path = fmt.Sprintf("/r/%s/search?%s", url.PathEscape(subreddit), q.Encode())
	}
	var resp listingResponse
	if err := c.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	return toDocs(resp.Data.Children, since, maxPosts)
}

func toDocs(children []struct {
	Data postData `json:"data"`
}, since time.Time, maxPosts int) ([]*types.Document, error) {
	out := make([]*types.Document, 0, len(children))
	for _, c := range children {
		created := time.Unix(int64(c.Data.CreatedUTC), 0).UTC()
		if created.Before(since) {
			continue
		}
		meta, _ := json.Marshal(map[string]any{
			"subreddit": c.Data.Subreddit,
			"flair":     c.Data.LinkFlairText,
		})
		u := c.Data.URL
		if u == "" || !isExternalURL(c.Data.Permalink) {
			u = redditPermalinkBase + c.Data.Permalink
		}
		out = append(out, &types.Document{
			Platform:         types.PlatformReddit,
			PlatformID:       c.Data.ID,
			Title:            c.Data.Title,
			Body:             c.Data.SelfText,
			Author:           c.Data.Author,
			Score:            c.Data.Score,
			URL:              u,
			CreatedAt:        created,
			FetchedAt:        time.Now().UTC(),
			PlatformMetadata: meta,
		})
		if maxPosts > 0 && len(out) >= maxPosts {
			break
		}
	}
	return out, nil
}

func isExternalURL(s string) bool {
	// permalinks start with "/r/", external URLs are absolute.
	return len(s) >= 4 && s[:4] == "http"
}

type commentChild struct {
	Kind string `json:"kind"`
	Data struct {
		ID         string  `json:"id"`
		Body       string  `json:"body"`
		Author     string  `json:"author"`
		Score      int     `json:"score"`
		CreatedUTC float64 `json:"created_utc"`
	} `json:"data"`
}

type commentsPage struct {
	Data struct {
		Children []commentChild `json:"children"`
	} `json:"data"`
}

// FetchTopComments returns at most `limit` top-sorted comments for the given post ID.
// Reddit's comments endpoint returns [postListing, commentListing]. We only read the second.
func FetchTopComments(ctx context.Context, c *Client, postID string, limit int) ([]*types.Reply, error) {
	path := fmt.Sprintf("/comments/%s.json?limit=%d&sort=top", url.PathEscape(postID), limit)
	var resp []commentsPage
	if err := c.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	if len(resp) < 2 {
		return nil, nil
	}
	out := make([]*types.Reply, 0, limit)
	for _, ch := range resp[1].Data.Children {
		if ch.Kind != "t1" {
			continue // skip 'more' and other kinds
		}
		out = append(out, &types.Reply{
			PlatformID: ch.Data.ID,
			Body:       ch.Data.Body,
			Author:     ch.Data.Author,
			Score:      ch.Data.Score,
			CreatedAt:  time.Unix(int64(ch.Data.CreatedUTC), 0).UTC(),
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
