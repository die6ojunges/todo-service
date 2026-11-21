package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"

	"github.com/google/uuid"

	"github.com/die6ojunges/todo-service/internal/todo"
)

// maxBodyBytes caps request bodies; a ToDo is tiny, so 1 MiB is generous and blocks memory abuse.
// errBodyTooLarge states this limit.
const maxBodyBytes = 1 << 20

// writableFields are the only keys a body may contain (errUnknownField names them).
var writableFields = []string{"title", "completed"}

// todoFields keeps the raw JSON per field so we can tell "absent" from "null" (needed for merge patch).
type todoFields map[string]json.RawMessage

// requireContentType returns unsupported unless the media type is one of allowed.
func requireContentType(r *http.Request, unsupported error, allowed ...string) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !slices.Contains(allowed, mediaType) {
		return unsupported
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
		return nil, errNotSingleObject // the body was JSON null
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errNotSingleObject
	}
	return fields, rejectUnknownFields(fields)
}

// classifyDecodeError maps a decoder failure to a fixed client error; the decoder's own
// message (Go type names, offsets) is never shown to clients.
func classifyDecodeError(err error) error {
	var tooLarge *http.MaxBytesError
	var notAnObject *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooLarge):
		return errBodyTooLarge
	case errors.As(err, &notAnObject):
		return errNotSingleObject // valid JSON, but an array, string or number
	default:
		return errMalformedJSON
	}
}

func rejectUnknownFields(fields todoFields) error {
	for name := range fields {
		if !slices.Contains(writableFields, name) {
			return errUnknownField
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
		return nil, errNullField
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errWrongType
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
		return todo.Draft{}, errTitleRequired
	}
	if patch.Completed == nil && requireCompleted {
		return todo.Draft{}, errCompletedRequired
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
		return uuid.Nil, errInvalidID
	}
	return id, nil
}
