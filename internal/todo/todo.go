// Package todo holds the ToDo domain: the model, its rules and the storage contract.
// It knows nothing about HTTP or SQL.
package todo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Todo is a single ToDo item.
type Todo struct {
	ID        uuid.UUID
	Title     string
	Completed bool
}

// ErrNotFound is returned when no item has the requested ID.
var ErrNotFound = errors.New("todo not found")

// ValidationError reports input that breaks a domain rule; callers map it to HTTP 422.
type ValidationError struct {
	Field string
	Value string
	Rule  string
}

// Error names the field, the offending value (when there is one) and the rule broken.
func (e *ValidationError) Error() string {
	if e.Value == "" {
		return fmt.Sprintf("invalid %s: %s", e.Field, e.Rule)
	}
	return fmt.Sprintf("invalid %s=%q: %s", e.Field, e.Value, e.Rule)
}

// Store persists ToDo items. Implementations must be safe for concurrent use.
type Store interface {
	// List returns every item in creation order; an empty store yields an empty, non-nil slice.
	List(ctx context.Context) ([]Todo, error)
	Get(ctx context.Context, id uuid.UUID) (Todo, error)
	Insert(ctx context.Context, item Todo) error
	// Update atomically loads the item, applies change and saves its result, so concurrent
	// read-modify-write requests (e.g. two PATCHes) never lose each other's changes.
	// If change returns an error, nothing is saved and that error is returned.
	Update(ctx context.Context, id uuid.UUID, change func(current Todo) (Todo, error)) (Todo, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
