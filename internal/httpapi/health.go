// Package httpapi exposes the service over HTTP: the ToDo REST API and health probes.
package httpapi

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

// readinessPingTimeout keeps /readyz well under the kubelet probe timeout (default 1s).
const readinessPingTimeout = 800 * time.Millisecond

// Pinger is any dependency whose availability gates readiness (e.g. the database pool).
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthChecks serves the Kubernetes probes. Rationale: README "Design decisions" §4.
type HealthChecks struct {
	dependency Pinger
	draining   atomic.Bool
}

// NewHealthChecks wires probes to the dependency that must be up for readiness.
//
//	health := httpapi.NewHealthChecks(dbPool)
//	mux.Handle("/", health.Routes())
func NewHealthChecks(dependency Pinger) *HealthChecks {
	return &HealthChecks{dependency: dependency}
}

// StartDraining makes /readyz fail so Kubernetes stops routing traffic before shutdown.
func (h *HealthChecks) StartDraining() {
	h.draining.Store(true)
}

// Routes returns a mux with GET /livez and GET /readyz.
func (h *HealthChecks) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", h.serveLiveness)
	mux.HandleFunc("GET /readyz", h.serveReadiness)
	return mux
}

// serveLiveness answers "is the process alive?" — deliberately checks nothing external.
func (h *HealthChecks) serveLiveness(w http.ResponseWriter, _ *http.Request) {
	writePlainStatus(w, http.StatusOK, "ok")
}

// serveReadiness answers "can this pod take traffic?".
func (h *HealthChecks) serveReadiness(w http.ResponseWriter, r *http.Request) {
	if h.draining.Load() {
		writePlainStatus(w, http.StatusServiceUnavailable, "draining")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), readinessPingTimeout)
	defer cancel()
	if err := h.dependency.Ping(ctx); err != nil {
		writePlainStatus(w, http.StatusServiceUnavailable, "dependency unavailable")
		return
	}
	writePlainStatus(w, http.StatusOK, "ok")
}

func writePlainStatus(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body + "\n"))
}
