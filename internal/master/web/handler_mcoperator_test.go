package web

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"

	"craftstack/internal/master/mcoperator"
)

// newTestServer builds a minimal Server suitable for handler-level tests.
// It skips DB/hub/auth — only the mcop field is populated.
func newTestServer(t *testing.T, opts MCOperatorOptions) *Server {
	t.Helper()
	return &Server{
		echo: echo.New(),
		log:  slog.Default(),
		mcop: opts,
	}
}

func TestHandleJenkinsForward_OK(t *testing.T) {
	var sawBody atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sawBody.Store(string(b))
		if r.Header.Get("Authorization") != "Bearer upstream-tok" {
			t.Errorf("upstream auth=%s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(202)
	}))
	defer upstream.Close()

	s := newTestServer(t, MCOperatorOptions{
		Client:             mcoperator.New(upstream.URL, "upstream-tok", slog.Default()),
		JenkinsSharedToken: "inbound-tok",
	})

	body, _ := json.Marshal(mcoperator.JenkinsTrigger{Server: "s1", Image: "i:1"})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/jenkins", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer inbound-tok")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	c := s.echo.NewContext(req, rr)

	if err := s.handleJenkinsForward(c); err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if rr.Code != http.StatusAccepted {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(sawBody.Load().(string), `"server":"s1"`) {
		t.Errorf("upstream body=%s", sawBody.Load())
	}
}

func TestHandleJenkinsForward_BadToken(t *testing.T) {
	s := newTestServer(t, MCOperatorOptions{
		Client:             mcoperator.New("http://unused", "x", slog.Default()),
		JenkinsSharedToken: "correct",
	})
	body, _ := json.Marshal(mcoperator.JenkinsTrigger{Server: "s", Image: "i"})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/jenkins", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong")
	rr := httptest.NewRecorder()
	c := s.echo.NewContext(req, rr)

	err := s.handleJenkinsForward(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 HTTPError, got %v", err)
	}
}

func TestHandleJenkinsForward_TokenViaQuery(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(202) }))
	defer upstream.Close()
	s := newTestServer(t, MCOperatorOptions{
		Client:             mcoperator.New(upstream.URL, "", slog.Default()),
		JenkinsSharedToken: "qtok",
	})
	body, _ := json.Marshal(mcoperator.JenkinsTrigger{Server: "s", Image: "i"})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/jenkins?token=qtok", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	c := s.echo.NewContext(req, rr)
	if err := s.handleJenkinsForward(c); err != nil {
		t.Fatalf("err: %v", err)
	}
	if rr.Code != http.StatusAccepted {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestHandleJenkinsForward_BadJSON(t *testing.T) {
	s := newTestServer(t, MCOperatorOptions{
		Client: mcoperator.New("http://x", "", slog.Default()),
	})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/jenkins", strings.NewReader("{not json"))
	rr := httptest.NewRecorder()
	c := s.echo.NewContext(req, rr)
	err := s.handleJenkinsForward(c)
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %v", err)
	}
}

func TestHandleJenkinsForward_MissingFields(t *testing.T) {
	s := newTestServer(t, MCOperatorOptions{Client: mcoperator.New("http://x", "", slog.Default())})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/jenkins", strings.NewReader(`{"server":"s"}`))
	rr := httptest.NewRecorder()
	c := s.echo.NewContext(req, rr)
	err := s.handleJenkinsForward(c)
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Errorf("expected 400 missing-field, got %v", err)
	}
}

func TestHandleJenkinsForward_UpstreamFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer upstream.Close()
	s := newTestServer(t, MCOperatorOptions{Client: mcoperator.New(upstream.URL, "", slog.Default())})
	body, _ := json.Marshal(mcoperator.JenkinsTrigger{Server: "s", Image: "i"})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/jenkins", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	c := s.echo.NewContext(req, rr)
	err := s.handleJenkinsForward(c)
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %v", err)
	}
}

func TestApiMCOperatorSync(t *testing.T) {
	var path atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path.Store(r.URL.Path)
		w.WriteHeader(202)
	}))
	defer upstream.Close()
	s := newTestServer(t, MCOperatorOptions{Client: mcoperator.New(upstream.URL, "", slog.Default())})

	req := httptest.NewRequest(http.MethodPost, "/api/mcoperator/sync/lobby", nil)
	rr := httptest.NewRecorder()
	c := s.echo.NewContext(req, rr)
	c.SetParamNames("server")
	c.SetParamValues("lobby")
	if err := s.apiMCOperatorSync(c); err != nil {
		t.Fatalf("err: %v", err)
	}
	if p, _ := path.Load().(string); p != "/api/v1/servers/lobby/sync" {
		t.Errorf("upstream path=%q", p)
	}
}

func TestApiMCOperatorSync_MissingName(t *testing.T) {
	s := newTestServer(t, MCOperatorOptions{Client: mcoperator.New("http://x", "", slog.Default())})
	req := httptest.NewRequest(http.MethodPost, "/api/mcoperator/sync/", nil)
	rr := httptest.NewRecorder()
	c := s.echo.NewContext(req, rr)
	c.SetParamNames("server")
	c.SetParamValues("")
	err := s.apiMCOperatorSync(c)
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %v", err)
	}
}

func TestApiMCOperatorServers(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"x"}]`))
	}))
	defer upstream.Close()
	s := newTestServer(t, MCOperatorOptions{Client: mcoperator.New(upstream.URL, "", slog.Default())})
	req := httptest.NewRequest(http.MethodGet, "/api/mcoperator/servers", nil)
	rr := httptest.NewRecorder()
	c := s.echo.NewContext(req, rr)
	if err := s.apiMCOperatorServers(c); err != nil {
		t.Fatalf("err: %v", err)
	}
	if rr.Body.String() != `[{"name":"x"}]` {
		t.Errorf("body=%s", rr.Body.String())
	}
}
