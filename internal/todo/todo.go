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

// Domain errors. Their messages are fixed and safe to show to API clients: they never include
// the offending value, so nothing a client sends is ever reflected back (see httpapi.publicErrors).
var (
	// ErrNotFound is returned when no item has the requested ID.
	ErrNotFound = errors.New("todo not found")
	// ErrTitleBlank and ErrTitleTooLong break the title rules; the HTTP layer maps them to 422.
	ErrTitleBlank   = errors.New("title must not be blank")
	ErrTitleTooLong = fmt.Errorf("title must be at most %d characters", MaxTitleLength)
)

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
