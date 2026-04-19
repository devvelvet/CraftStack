package observability

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInfluxPushOnce(t *testing.T) {
	var gotBody atomic.Value
	var gotAuth atomic.Value
	var gotURL atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody.Store(string(b))
		gotAuth.Store(r.Header.Get("Authorization"))
		gotURL.Store(r.URL.String())
		w.WriteHeader(204)
	}))
	defer srv.Close()

	src := &fakeSource{
		nodes: []NodeSnapshot{{NodeID: "n 1", CPUPercent: 50, MemPercent: 60, MemUsedMB: 1, MemTotalMB: 2, DiskPercent: 3}},
		insts: []InstanceSnapshot{{InstanceID: "i,1", NodeID: "n 1", Type: "redis", CPUPercent: 5, MemoryUsedMB: 10, MemoryLimitMB: 20, NetRxBytes: 30, NetTxBytes: 40, BlockReadBytes: 50, BlockWriteBytes: 60}},
	}
	p := NewInfluxPusher(srv.URL, "tok", "org1", "buck1", 0, src, slog.Default())
	if err := p.pushOnce(context.Background()); err != nil {
		t.Fatalf("pushOnce: %v", err)
	}

	body := gotBody.Load().(string)
	if !strings.Contains(body, `craftstack_node,node=n\ 1 cpu_percent=50`) {
		t.Errorf("node line missing/escaping wrong: %s", body)
	}
	if !strings.Contains(body, `craftstack_instance,instance=i\,1,node=n\ 1,type=redis`) {
		t.Errorf("instance line missing/escaping wrong: %s", body)
	}
	if gotAuth.Load().(string) != "Token tok" {
		t.Errorf("auth header: %v", gotAuth.Load())
	}
	if u := gotURL.Load().(string); !strings.Contains(u, "org=org1") || !strings.Contains(u, "bucket=buck1") {
		t.Errorf("url missing params: %s", u)
	}
}

func TestInfluxPushOnceEmpty(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	p := NewInfluxPusher(srv.URL, "t", "o", "b", 0, &fakeSource{}, slog.Default())
	if err := p.pushOnce(context.Background()); err != nil {
		t.Fatalf("empty push: %v", err)
	}
	if called {
		t.Errorf("should skip HTTP when no data")
	}
}

func TestInfluxPushErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte("bad token"))
	}))
	defer srv.Close()
	src := &fakeSource{nodes: []NodeSnapshot{{NodeID: "n"}}}
	p := NewInfluxPusher(srv.URL, "t", "o", "b", 0, src, slog.Default())
	err := p.pushOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 error, got %v", err)
	}
}

func TestInfluxPusherRunStopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	p := NewInfluxPusher(srv.URL, "t", "o", "b", 10*time.Millisecond, &fakeSource{}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit on cancel")
	}
}

func TestEscapeTag(t *testing.T) {
	cases := map[string]string{
		"plain":   "plain",
		"a b":     `a\ b`,
		"a,b":     `a\,b`,
		"a=b":     `a\=b`,
		"a,b c=d": `a\,b\ c\=d`,
	}
	for in, want := range cases {
		if got := escapeTag(in); got != want {
			t.Errorf("escapeTag(%q)=%q want %q", in, got, want)
		}
	}
}
