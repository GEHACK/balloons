// Package loom talks to loom.gehack.nl, the contest's seating service. It
// fetches per-team map images, embedded on every printed ticket so the runner
// can see where the team sits without consulting a separate seating chart,
// and resolves station addresses to team names for cuproxy print jobs.
package loom

import (
	"context"
	"encoding/json"
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

// TeamName returns the display name of the team seated at a station address,
// via `<baseURL>/api/team-info/<ip>`. Returns "" with a nil error when loom
// knows no team at that address (404), so callers can treat "unseated" as
// "unknown" rather than as a failure.
func (c *Client) TeamName(ctx context.Context, ip string) (string, error) {
	u := c.baseURL + "/api/team-info/" + url.PathEscape(ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("loom: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("loom: GET %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("loom: GET %s: status %d", u, resp.StatusCode)
	}
	var info struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("loom: decode team-info: %w", err)
	}
	return info.Name, nil
}
