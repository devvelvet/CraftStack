// Package mcoperator integrates CraftStack with a devvelvet/mc-operator daemon.
//
// The operator exposes a REST API (/api/v1/...) and an SSE event stream. This
// client covers the subset CraftStack needs:
//
//   - POST /api/v1/triggers/jenkins  — forward a Jenkins build as a deploy
//   - POST /api/v1/servers/{name}/sync — trigger a sync pipeline
//   - GET  /api/v1/servers            — snapshot all servers
//   - GET  /api/v1/events             — SSE stream for live deploy events
//
// See docs/mc-operator-integration.md for wiring and auth.
package mcoperator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Client talks to a mc-operator instance.
type Client struct {
	BaseURL string // e.g. http://mc-operator:8080
	Token   string // Bearer token for authenticated endpoints
	Log     *slog.Logger
	http    *http.Client
}

// New returns a Client with a 10s default timeout.
func New(baseURL, token string, log *slog.Logger) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		Log:     log,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// JenkinsTrigger matches the payload mc-operator expects on
// POST /api/v1/triggers/jenkins.
type JenkinsTrigger struct {
	Server        string `json:"server"`
	Image         string `json:"image"`
	Revision      string `json:"revision,omitempty"`
	BuildID       string `json:"buildId,omitempty"`
	JobName       string `json:"jobName,omitempty"`
	Strict        bool   `json:"strict,omitempty"`
	ConfigOverlay bool   `json:"configOverlay,omitempty"`
}

// TriggerJenkins forwards a Jenkins build result to mc-operator.
func (c *Client) TriggerJenkins(ctx context.Context, t JenkinsTrigger) error {
	body, err := json.Marshal(t)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/api/v1/triggers/jenkins", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("mc-operator trigger status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// Sync kicks the JAR pipeline for a single server (manual sync).
func (c *Client) Sync(ctx context.Context, server string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/api/v1/servers/"+server+"/sync", nil)
	if err != nil {
		return err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("mc-operator sync status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// Servers returns the raw JSON snapshot from /api/v1/servers. Callers decode
// as needed so schema drift in the operator doesn't break the client.
func (c *Client) Servers(ctx context.Context) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/servers", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("mc-operator servers status %d", resp.StatusCode)
	}
	return body, nil
}

// Event is one parsed line from the SSE stream.
type Event struct {
	Event string
	Data  string
}

// FollowEvents streams /api/v1/events, invoking onEvent for each. It reconnects
// with backoff until ctx is canceled.
func (c *Client) FollowEvents(ctx context.Context, onEvent func(Event)) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.streamOnce(ctx, onEvent)
		if ctx.Err() != nil {
			return
		}
		c.Log.Warn("mc-operator event stream ended; reconnecting", "error", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) streamOnce(ctx context.Context, onEvent func(Event)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	// No http.Client timeout — SSE is long-lived.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sse status %d", resp.StatusCode)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var ev Event
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if ev.Event != "" || ev.Data != "" {
				onEvent(ev)
			}
			ev = Event{}
		case strings.HasPrefix(line, "event:"):
			ev.Event = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			if ev.Data != "" {
				ev.Data += "\n"
			}
			ev.Data += strings.TrimSpace(line[len("data:"):])
		}
	}
	return sc.Err()
}
