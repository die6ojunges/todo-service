package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverPanicsReturns500AndLogs(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })

	rec := httptest.NewRecorder()
	recoverPanics(panicking, logger).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/todos", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("response leaks panic value: %s", rec.Body)
	}
	if !strings.Contains(logs.String(), `"panic":"boom"`) {
		t.Errorf("logs = %s, want the panic value recorded", logs.String())
	}
}
