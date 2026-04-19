package mcoperator

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTriggerJenkins(t *testing.T) {
	var body atomic.Value
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/triggers/jenkins" || r.Method != http.MethodPost {
			t.Errorf("bad req: %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		body.Store(string(b))
		auth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	c := New(srv.URL, "tok-1", slog.Default())
	err := c.TriggerJenkins(context.Background(), JenkinsTrigger{
		Server: "s1", Image: "img:1", BuildID: "42", Strict: true,
	})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if auth.Load().(string) != "Bearer tok-1" {
		t.Errorf("auth=%v", auth.Load())
	}
	var got JenkinsTrigger
	if err := json.Unmarshal([]byte(body.Load().(string)), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.Server != "s1" || got.Image != "img:1" || !got.Strict {
		t.Errorf("payload mismatch: %+v", got)
	}
}

func TestTriggerJenkinsErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		_, _ = w.Write([]byte("image drift"))
	}))
	defer srv.Close()
	c := New(srv.URL, "", slog.Default())
	err := c.TriggerJenkins(context.Background(), JenkinsTrigger{Server: "s", Image: "i"})
	if err == nil || !strings.Contains(err.Error(), "409") {
		t.Errorf("want 409 err got %v", err)
	}
}

func TestSync(t *testing.T) {
	var path atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path.Store(r.URL.Path)
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		w.WriteHeader(202)
	}))
	defer srv.Close()
	c := New(srv.URL, "", slog.Default())
	if err := c.Sync(context.Background(), "survival"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if path.Load().(string) != "/api/v1/servers/survival/sync" {
		t.Errorf("path=%v", path.Load())
	}
}

func TestServers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/servers" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"name":"a"}]`))
	}))
	defer srv.Close()
	c := New(srv.URL, "", slog.Default())
	raw, err := c.Servers(context.Background())
	if err != nil {
		t.Fatalf("servers: %v", err)
	}
	if string(raw) != `[{"name":"a"}]` {
		t.Errorf("body=%s", string(raw))
	}
}

func TestServersErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := New(srv.URL, "", slog.Default())
	if _, err := c.Servers(context.Background()); err == nil {
		t.Errorf("expected error")
	}
}

func TestFollowEventsParses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("event: deploy\ndata: {\"ok\":true}\n\nevent: log\ndata: hello\n\n"))
		flusher.Flush()
		// hold briefly so client can read before close
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	c := New(srv.URL, "", slog.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got := make(chan Event, 4)
	go c.FollowEvents(ctx, func(e Event) {
		select {
		case got <- e:
		default:
		}
	})

	want := []Event{{Event: "deploy", Data: `{"ok":true}`}, {Event: "log", Data: "hello"}}
	for _, w := range want {
		select {
		case e := <-got:
			if e != w {
				t.Errorf("event mismatch: got %+v want %+v", e, w)
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for %+v", w)
		}
	}
}
