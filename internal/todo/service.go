package todo

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxTitleLength is the maximum title size, in characters (not bytes).
const MaxTitleLength = 200

// IDGenerator creates IDs for new items; injected so tests can use deterministic IDs.
type IDGenerator func() uuid.UUID

// Draft is a complete representation of an item as sent by a client (POST and PUT).
type Draft struct {
	Title     string
	Completed bool
}

// Patch is a partial update (JSON Merge Patch): nil fields are left unchanged.
type Patch struct {
	Title     *string
	Completed *bool
}

// Service applies the ToDo rules on top of a Store.
type Service struct {
	store Store
	newID IDGenerator
}

// NewService wires the rules to a storage backend.
//
//	svc := todo.NewService(store, uuid.New)
//	item, err := svc.Create(ctx, todo.Draft{Title: "buy milk"})
func NewService(store Store, newID IDGenerator) *Service {
	return &Service{store: store, newID: newID}
}

// List returns all items in creation order.
func (s *Service) List(ctx context.Context) ([]Todo, error) {
	return s.store.List(ctx)
}

// Get returns one item or ErrNotFound.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Todo, error) {
	return s.store.Get(ctx, id)
}

// Create validates the draft and stores it under a new server-generated ID.
func (s *Service) Create(ctx context.Context, draft Draft) (Todo, error) {
	title, err := normalizeTitle(draft.Title)
	if err != nil {
		return Todo{}, err
	}
	item := Todo{ID: s.newID(), Title: title, Completed: draft.Completed}
	if err := s.store.Insert(ctx, item); err != nil {
		return Todo{}, fmt.Errorf("insert todo %s: %w", item.ID, err)
	}
	return item, nil
}

// Replace overwrites every field of an existing item (PUT semantics; never creates).
func (s *Service) Replace(ctx context.Context, id uuid.UUID, draft Draft) (Todo, error) {
	title, err := normalizeTitle(draft.Title)
	if err != nil {
		return Todo{}, err
	}
	return s.store.Update(ctx, id, func(current Todo) (Todo, error) {
		return Todo{ID: current.ID, Title: title, Completed: draft.Completed}, nil
	})
}

// Patch changes only the fields present in patch (PATCH semantics).
func (s *Service) Patch(ctx context.Context, id uuid.UUID, patch Patch) (Todo, error) {
	return s.store.Update(ctx, id, func(current Todo) (Todo, error) {
		return applyPatch(current, patch)
	})
}

// Delete removes an item or returns ErrNotFound.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.store.Delete(ctx, id)
}

func applyPatch(current Todo, patch Patch) (Todo, error) {
	if patch.Title != nil {
		title, err := normalizeTitle(*patch.Title)
		if err != nil {
			return Todo{}, err
		}
		current.Title = title
	}
	if patch.Completed != nil {
		current.Completed = *patch.Completed
	}
	return current, nil
}

// normalizeTitle trims surrounding whitespace and enforces 1..MaxTitleLength characters.
func normalizeTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", fmt.Errorf("%w title=%q: must not be empty or blank", ErrInvalid, raw)
	}
	if utf8.RuneCountInString(title) > MaxTitleLength {
		return "", fmt.Errorf("%w title=%q: must be at most %d characters", ErrInvalid, truncateForError(title), MaxTitleLength)
	}
	return title, nil
}

// truncateForError keeps error messages readable when the offending value is huge.
func truncateForError(value string) string {
	const shown = 40
	if utf8.RuneCountInString(value) <= shown {
		return value
	}
	return string([]rune(value)[:shown]) + "…"
}
