package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/die6ojunges/todo-service/internal/todo"
)

// maxBodyBytes caps request bodies; a ToDo is tiny, so 1 MiB is generous and blocks memory abuse.
const maxBodyBytes = 1 << 20

var writableFields = []string{"title", "completed"}

// todoFields keeps the raw JSON per field so we can tell "absent" from "null" (needed for merge patch).
type todoFields map[string]json.RawMessage

func requireContentType(r *http.Request, allowed ...string) error {
	header := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil || !slices.Contains(allowed, mediaType) {
		return &requestError{
			status: http.StatusUnsupportedMediaType,
			detail: fmt.Sprintf("Content-Type %q is not supported; want %s", header, strings.Join(allowed, " or ")),
		}
	}
	return nil
}

// decodeFields reads exactly one JSON object containing only writable fields.
func decodeFields(w http.ResponseWriter, r *http.Request) (todoFields, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	var fields todoFields
	if err := decoder.Decode(&fields); err != nil {
		return nil, classifyDecodeError(err)
	}
	if fields == nil {
		return nil, badRequest("request body must be a JSON object, got null")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, badRequest("request body must contain a single JSON object")
	}
	return fields, rejectUnknownFields(fields)
}

func classifyDecodeError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return &requestError{
			status: http.StatusRequestEntityTooLarge,
			detail: fmt.Sprintf("request body exceeds %d bytes", tooLarge.Limit),
		}
	}
	return badRequest("malformed JSON body: %v", err)
}

func rejectUnknownFields(fields todoFields) error {
	for name := range fields {
		if !slices.Contains(writableFields, name) {
			return badRequest("unknown field %q; writable fields are %s", name, strings.Join(writableFields, ", "))
		}
	}
	return nil
}

// readField returns nil when the field is absent. Null is rejected: in JSON Merge Patch it
// means "remove", and every ToDo field is required.
func readField[T any](fields todoFields, name string) (*T, error) {
	raw, present := fields[name]
	if !present {
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, fmt.Errorf("%w %s=null: must not be null", todo.ErrInvalid, name)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, badRequest("field %q has the wrong type (got %s): %v", name, raw, err)
	}
	return &value, nil
}

// toPatch converts fields to a partial update.
func (f todoFields) toPatch() (todo.Patch, error) {
	title, err := readField[string](f, "title")
	if err != nil {
		return todo.Patch{}, err
	}
	completed, err := readField[bool](f, "completed")
	if err != nil {
		return todo.Patch{}, err
	}
	return todo.Patch{Title: title, Completed: completed}, nil
}

// toDraft converts fields to a full representation. title is always required;
// completed is required for PUT (full replacement) and defaults to false for POST.
func (f todoFields) toDraft(requireCompleted bool) (todo.Draft, error) {
	patch, err := f.toPatch()
	if err != nil {
		return todo.Draft{}, err
	}
	if patch.Title == nil {
		return todo.Draft{}, fmt.Errorf("%w title: is required", todo.ErrInvalid)
	}
	if patch.Completed == nil && requireCompleted {
		return todo.Draft{}, fmt.Errorf("%w completed: is required for a full replacement (PUT)", todo.ErrInvalid)
	}
	draft := todo.Draft{Title: *patch.Title}
	if patch.Completed != nil {
		draft.Completed = *patch.Completed
	}
	return draft, nil
}

func parseID(r *http.Request) (uuid.UUID, error) {
	raw := r.PathValue("id")
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, badRequest("invalid id %q: want a UUID like 3f1c9a52-6a0e-4b8e-9a51-0d2b6f1e7c11", raw)
	}
	return id, nil
}
