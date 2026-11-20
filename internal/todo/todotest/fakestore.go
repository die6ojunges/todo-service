package todotest

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/google/uuid"

	"github.com/die6ojunges/todo-service/internal/todo"
)

// FakeStore is an in-memory todo.Store for fast, dependency-free tests of the layers above
// storage; the running service uses PostgreSQL (README "Design decisions" §1). It passes the
// same RunStoreContract as the PostgreSQL store, so tests built on it can trust its behavior.
// It keeps items in a slice to preserve insertion order; a mutex makes it concurrency-safe.
type FakeStore struct {
	mu    sync.Mutex
	items []todo.Todo
}

// NewFakeStore returns an empty store.
//
//	svc := todo.NewService(todotest.NewFakeStore(), uuid.New)
func NewFakeStore() *FakeStore {
	return &FakeStore{items: []todo.Todo{}}
}

// List returns a copy so callers can't mutate the store's state.
func (s *FakeStore) List(context.Context) ([]todo.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.items), nil
}

// Get returns the item with id or todo.ErrNotFound.
func (s *FakeStore) Get(_ context.Context, id uuid.UUID) (todo.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.indexOf(id)
	if err != nil {
		return todo.Todo{}, err
	}
	return s.items[index], nil
}

// Insert appends a new item; a duplicate ID is a programming error and is rejected.
func (s *FakeStore) Insert(_ context.Context, item todo.Todo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.indexOf(item.ID); err == nil {
		return fmt.Errorf("insert todo: id %s already exists, want a unique id", item.ID)
	}
	s.items = append(s.items, item)
	return nil
}

// Update holds the lock across read-change-write, which makes it atomic.
func (s *FakeStore) Update(_ context.Context, id uuid.UUID, change func(todo.Todo) (todo.Todo, error)) (todo.Todo, error) {
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
func (s *FakeStore) Delete(_ context.Context, id uuid.UUID) error {
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
func (s *FakeStore) indexOf(id uuid.UUID) (int, error) {
	index := slices.IndexFunc(s.items, func(item todo.Todo) bool { return item.ID == id })
	if index < 0 {
		return -1, fmt.Errorf("%w: id %s", todo.ErrNotFound, id)
	}
	return index, nil
}
