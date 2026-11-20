package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/die6ojunges/todo-service/internal/todo"
)

// problem is an RFC 9457 Problem Details body.
type problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// requestError is a client mistake detected by the HTTP layer (400, 413, 415).
type requestError struct {
	status int
	detail string
}

func (e *requestError) Error() string { return e.detail }

func badRequest(format string, args ...any) error {
	return &requestError{status: http.StatusBadRequest, detail: fmt.Sprintf(format, args...)}
}

// writeError maps any error to a Problem Details response; unknown errors become 500
// and are logged, never leaked to the client.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var reqErr *requestError
	switch {
	case errors.As(err, &reqErr):
		writeProblem(w, r, reqErr.status, reqErr.detail)
	case errors.Is(err, todo.ErrInvalid):
		writeProblem(w, r, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, todo.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, err.Error())
	default:
		logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err.Error())
		writeProblem(w, r, http.StatusInternalServerError, "unexpected error; see server logs")
	}
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	body := problem{
		Type:     "about:blank",
		Title:    http.StatusText(status),
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
	}
	writeBody(w, status, "application/problem+json", body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	writeBody(w, status, "application/json", body)
}

func writeBody(w http.ResponseWriter, status int, contentType string, body any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	// Headers are already sent; an encode error here means the client went away.
	_ = json.NewEncoder(w).Encode(body)
}
