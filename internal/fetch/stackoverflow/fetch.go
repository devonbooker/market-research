package stackoverflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/devonbooker/market-research/internal/types"
)

type question struct {
	QuestionID       int64    `json:"question_id"`
	Title            string   `json:"title"`
	Body             string   `json:"body"`
	Owner            owner    `json:"owner"`
	Score            int      `json:"score"`
	ViewCount        int      `json:"view_count"`
	Tags             []string `json:"tags"`
	Link             string   `json:"link"`
	CreationDate     int64    `json:"creation_date"`
	IsAnswered       bool     `json:"is_answered"`
	AcceptedAnswerID *int64   `json:"accepted_answer_id,omitempty"`
}

type answer struct {
	AnswerID     int64  `json:"answer_id"`
	QuestionID   int64  `json:"question_id"`
	Body         string `json:"body"`
	Owner        owner  `json:"owner"`
	Score        int    `json:"score"`
	IsAccepted   bool   `json:"is_accepted"`
	CreationDate int64  `json:"creation_date"`
}

type owner struct {
	DisplayName string `json:"display_name"`
}

type itemsResponse[T any] struct {
	Items []T `json:"items"`
}

// FetchQuestionsByTag pulls new questions for a single tag since the given time, up to maxPosts.
// Uses filter=withbody to get question body inline.
func FetchQuestionsByTag(ctx context.Context, c *Client, tag string, since time.Time, maxPosts int) ([]*types.Document, error) {
	params := url.Values{}
	params.Set("tagged", tag)
	params.Set("fromdate", strconv.FormatInt(since.Unix(), 10))
	params.Set("sort", "creation")
	params.Set("order", "desc")
	params.Set("pagesize", "100")
	params.Set("filter", "withbody")

	var resp itemsResponse[question]
	if err := c.GetJSON(ctx, "/questions", params, &resp); err != nil {
		return nil, err
	}
	return questionsToDocs(resp.Items, maxPosts), nil
}

// FetchSearch runs an advanced search query.
func FetchSearch(ctx context.Context, c *Client, query string, since time.Time, maxPosts int) ([]*types.Document, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("fromdate", strconv.FormatInt(since.Unix(), 10))
	params.Set("sort", "creation")
	params.Set("order", "desc")
	params.Set("pagesize", "100")
	params.Set("filter", "withbody")

	var resp itemsResponse[question]
	if err := c.GetJSON(ctx, "/search/advanced", params, &resp); err != nil {
		return nil, err
	}
	return questionsToDocs(resp.Items, maxPosts), nil
}

// FetchAcceptedAnswer fetches a specific answer by ID. Returns nil reply if not accepted.
func FetchAcceptedAnswer(ctx context.Context, c *Client, answerID int64) (*types.Reply, error) {
	params := url.Values{}
	params.Set("filter", "withbody")
	path := fmt.Sprintf("/answers/%d", answerID)

	var resp itemsResponse[answer]
	if err := c.GetJSON(ctx, path, params, &resp); err != nil {
		return nil, err
	}
	if len(resp.Items) == 0 {
		return nil, nil
	}
	a := resp.Items[0]
	accepted := a.IsAccepted
	return &types.Reply{
		PlatformID: strconv.FormatInt(a.AnswerID, 10),
		Body:       a.Body,
		Author:     a.Owner.DisplayName,
		Score:      a.Score,
		CreatedAt:  time.Unix(a.CreationDate, 0).UTC(),
		IsAccepted: &accepted,
	}, nil
}

func questionsToDocs(qs []question, maxPosts int) []*types.Document {
	out := make([]*types.Document, 0, len(qs))
	for _, q := range qs {
		meta, _ := json.Marshal(map[string]any{
			"tags":               q.Tags,
			"view_count":         q.ViewCount,
			"is_answered":        q.IsAnswered,
			"accepted_answer_id": q.AcceptedAnswerID,
		})
		out = append(out, &types.Document{
			Platform:         types.PlatformStackOverflow,
			PlatformID:       strconv.FormatInt(q.QuestionID, 10),
			Title:            q.Title,
			Body:             q.Body,
			Author:           q.Owner.DisplayName,
			Score:            q.Score,
			URL:              q.Link,
			CreatedAt:        time.Unix(q.CreationDate, 0).UTC(),
			FetchedAt:        time.Now().UTC(),
			PlatformMetadata: meta,
		})
		if maxPosts > 0 && len(out) >= maxPosts {
			break
		}
	}
	return out
}
