package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/die6ojunges/todo-service/internal/todo"
)

// problem is an RFC 9457 Problem Details body. "type" is omitted because it defaults to
// about:blank ("the status code says it all"), and "instance" because it would echo the path.
type problem struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// Client errors detected by the HTTP layer. Like the domain errors, their messages are fixed:
// nothing from the request (values, field names, headers, path) and no parser internals are
// ever reflected back, which rules out injected content and fingerprinting.
var (
	errInvalidID         = errors.New("id must be a UUID")
	errMalformedJSON     = errors.New("request body must be valid JSON")
	errNotSingleObject   = errors.New("request body must be a single JSON object")
	errUnknownField      = errors.New("request body may only contain the fields title and completed")
	errWrongType         = errors.New("title must be a string and completed must be a boolean")
	errNullField         = errors.New("title and completed must not be null")
	errTitleRequired     = errors.New("title is required")
	errCompletedRequired = errors.New("completed is required when replacing a todo")
	errBodyTooLarge      = errors.New("request body must be at most 1 MiB") // matches maxBodyBytes
	errNotJSON           = errors.New("content type must be application/json")
	errNotPatchJSON      = errors.New("content type must be application/merge-patch+json or application/json")
)

// publicError pairs a known error with its HTTP status; the error's own message is the detail.
type publicError struct {
	err    error
	status int
}

// publicErrors is the complete client-facing error contract. An error not listed here becomes
// a 500 whose cause is logged but never returned.
var publicErrors = []publicError{
	{errInvalidID, http.StatusBadRequest},
	{errMalformedJSON, http.StatusBadRequest},
	{errNotSingleObject, http.StatusBadRequest},
	{errUnknownField, http.StatusBadRequest},
	{errWrongType, http.StatusBadRequest},
	{todo.ErrNotFound, http.StatusNotFound},
	{errBodyTooLarge, http.StatusRequestEntityTooLarge},
	{errNotJSON, http.StatusUnsupportedMediaType},
	{errNotPatchJSON, http.StatusUnsupportedMediaType},
	{errNullField, http.StatusUnprocessableEntity},
	{errTitleRequired, http.StatusUnprocessableEntity},
	{errCompletedRequired, http.StatusUnprocessableEntity},
	{todo.ErrTitleBlank, http.StatusUnprocessableEntity},
	{todo.ErrTitleTooLong, http.StatusUnprocessableEntity},
}

// internalErrorDetail is all a client learns about a server-side failure.
const internalErrorDetail = "internal server error"

// writeError answers with the fixed message of a known error, or a logged 500 otherwise.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	for _, known := range publicErrors {
		if errors.Is(err, known.err) {
			writeProblem(w, known.status, known.err.Error())
			return
		}
	}
	logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err.Error())
	writeProblem(w, http.StatusInternalServerError, internalErrorDetail)
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	body := problem{Title: http.StatusText(status), Status: status, Detail: detail}
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
