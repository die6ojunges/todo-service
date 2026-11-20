package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeDependency is a named fake for a downstream dependency (e.g. the database).
type fakeDependency struct{ err error }

func (f fakeDependency) Ping(context.Context) error { return f.err }

func getStatus(t *testing.T, h http.Handler, path string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec.Code
}

func TestLivezIsOKEvenWhenDependencyIsDown(t *testing.T) {
	// Liveness must not check the DB: a DB outage would restart every replica in a loop.
	health := NewHealthChecks(fakeDependency{err: errors.New("db down")})
	if got := getStatus(t, health.Routes(), "/livez"); got != http.StatusOK {
		t.Errorf("GET /livez = %d, want 200", got)
	}
}

func TestReadyzIsOKWhenDependencyIsUp(t *testing.T) {
	health := NewHealthChecks(fakeDependency{})
	if got := getStatus(t, health.Routes(), "/readyz"); got != http.StatusOK {
		t.Errorf("GET /readyz = %d, want 200", got)
	}
}

func TestReadyzIsUnavailableWhenDependencyIsDown(t *testing.T) {
	health := NewHealthChecks(fakeDependency{err: errors.New("db down")})
	if got := getStatus(t, health.Routes(), "/readyz"); got != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz = %d, want 503", got)
	}
}

func TestReadyzIsUnavailableWhileDraining(t *testing.T) {
	health := NewHealthChecks(fakeDependency{})
	health.StartDraining()
	if got := getStatus(t, health.Routes(), "/readyz"); got != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz after StartDraining = %d, want 503", got)
	}
	if got := getStatus(t, health.Routes(), "/livez"); got != http.StatusOK {
		t.Errorf("GET /livez after StartDraining = %d, want 200", got)
	}
}

func TestHealthRoutesRejectNonGetMethods(t *testing.T) {
	health := NewHealthChecks(fakeDependency{})
	rec := httptest.NewRecorder()
	health.Routes().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/livez", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /livez = %d, want 405", rec.Code)
	}
}
