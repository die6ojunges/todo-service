package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// NewRouter mounts every route of the service behind panic recovery.
//
//	router := httpapi.NewRouter(health, httpapi.NewTodoAPI(service, logger), logger)
func NewRouter(health *HealthChecks, todos *TodoAPI, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	health.register(mux)
	todos.register(mux)
	return recoverPanics(mux, logger)
}

// recoverPanics turns a handler panic into a logged 500 instead of a dropped connection,
// so one bad request never takes the pod's readiness or other requests down with it.
func recoverPanics(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			if recovered == http.ErrAbortHandler { //nolint:errorlint // sentinel compared by identity, as net/http does
				panic(recovered) // net/http's own signal to abort the response; must propagate
			}
			logger.Error("panic serving request", "method", r.Method, "path", r.URL.Path,
				"panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
			writeProblem(w, r, http.StatusInternalServerError, "unexpected error; see server logs")
		}()
		next.ServeHTTP(w, r)
	})
}
