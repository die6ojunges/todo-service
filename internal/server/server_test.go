package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/die6ojunges/todo-service/internal/httpapi"
)

type alwaysUp struct{}

func (alwaysUp) Ping(context.Context) error { return nil }

func startTestServer(t *testing.T, drain time.Duration) (baseURL string, stop context.CancelFunc, done <-chan error) {
	t.Helper()
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	health := httpapi.NewHealthChecks(alwaysUp{})
	srv := New(health.Routes(), health, Timeouts{Drain: drain, Shutdown: time.Second},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ctx, listener) }()
	return "http://" + listener.Addr().String(), cancel, errs
}

func statusOf(t *testing.T, url string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request for %s: %v", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func TestServeAnswersUntilContextIsCancelled(t *testing.T) {
	baseURL, stop, done := startTestServer(t, 0)
	if got := statusOf(t, baseURL+"/livez"); got != http.StatusOK {
		t.Fatalf("GET /livez = %d, want 200", got)
	}
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() returned %v, want nil on graceful shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve() did not return after context cancel")
	}
}

func TestServeReportsNotReadyWhileDraining(t *testing.T) {
	baseURL, stop, done := startTestServer(t, 300*time.Millisecond)
	stop()
	time.Sleep(50 * time.Millisecond) // let Serve observe the cancellation and start draining
	if got := statusOf(t, baseURL+"/readyz"); got != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz during drain = %d, want 503", got)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve() returned %v, want nil", err)
	}
}
