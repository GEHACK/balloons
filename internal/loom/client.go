// Package loom fetches per-team map images from loom.gehack.nl. The map is
// embedded on every printed ticket so the runner can see where the team sits
// in the contest area without consulting a separate seating chart.
package loom

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Fetch returns the PNG bytes of the team's map image. The endpoint is
// `<baseURL>/api/map-image?team_id=<id>`.
func (c *Client) Fetch(ctx context.Context, teamID string) ([]byte, error) {
	u := c.baseURL + "/api/map-image?team_id=" + url.QueryEscape(teamID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("loom: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loom: GET %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loom: GET %s: status %d", u, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("loom: read body: %w", err)
	}
	return body, nil
}
