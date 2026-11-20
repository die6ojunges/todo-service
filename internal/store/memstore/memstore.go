// Package memstore is an in-memory todo.Store. It exists for fast, dependency-free tests;
// the running service uses PostgreSQL (see README "Design decisions" §1).
package memstore

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/google/uuid"

	"github.com/die6ojunges/todo-service/internal/todo"
)

// Store keeps items in a slice to preserve insertion order; a mutex makes it concurrency-safe.
type Store struct {
	mu    sync.Mutex
	items []todo.Todo
}

// New returns an empty store.
//
//	svc := todo.NewService(memstore.New(), uuid.New)
func New() *Store {
	return &Store{items: []todo.Todo{}}
}

// List returns a copy so callers can't mutate the store's state.
func (s *Store) List(context.Context) ([]todo.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.items), nil
}

// Get returns the item with id or todo.ErrNotFound.
func (s *Store) Get(_ context.Context, id uuid.UUID) (todo.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.indexOf(id)
	if err != nil {
		return todo.Todo{}, err
	}
	return s.items[index], nil
}

// Insert appends a new item; a duplicate ID is a programming error and is rejected.
func (s *Store) Insert(_ context.Context, item todo.Todo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.indexOf(item.ID); err == nil {
		return fmt.Errorf("insert todo: id %s already exists, want a unique id", item.ID)
	}
	s.items = append(s.items, item)
	return nil
}

// Update holds the lock across read-change-write, which makes it atomic.
func (s *Store) Update(_ context.Context, id uuid.UUID, change func(todo.Todo) (todo.Todo, error)) (todo.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.indexOf(id)
	if err != nil {
		return todo.Todo{}, err
	}
	updated, err := change(s.items[index])
	if err != nil {
		return todo.Todo{}, err
	}
	updated.ID = id // the ID is immutable whatever change returns
	s.items[index] = updated
	return updated, nil
}

// Delete removes the item with id or returns todo.ErrNotFound.
func (s *Store) Delete(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.indexOf(id)
	if err != nil {
		return err
	}
	s.items = slices.Delete(s.items, index, index+1)
	return nil
}

// indexOf must be called with mu held.
func (s *Store) indexOf(id uuid.UUID) (int, error) {
	index := slices.IndexFunc(s.items, func(item todo.Todo) bool { return item.ID == id })
	if index < 0 {
		return -1, fmt.Errorf("%w: id %s", todo.ErrNotFound, id)
	}
	return index, nil
}
