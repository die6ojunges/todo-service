// Package todotest holds a reusable contract suite every todo.Store implementation must pass,
// so FakeStore and the PostgreSQL store are guaranteed to behave identically.
package todotest

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/die6ojunges/todo-service/internal/todo"
)

// StoreFactory returns a fresh, empty store for one subtest.
type StoreFactory func(t *testing.T) todo.Store

// RunStoreContract runs the full contract against stores built by newStore.
//
//	func TestFakeStore(t *testing.T) { todotest.RunStoreContract(t, func(*testing.T) todo.Store { return todotest.NewFakeStore() }) }
func RunStoreContract(t *testing.T, newStore StoreFactory) {
	t.Run("list empty is non-nil", func(t *testing.T) { testListEmpty(t, newStore(t)) })
	t.Run("list keeps insertion order", func(t *testing.T) { testListOrder(t, newStore(t)) })
	t.Run("get returns inserted item", func(t *testing.T) { testGet(t, newStore(t)) })
	t.Run("unknown id is not found", func(t *testing.T) { testNotFound(t, newStore(t)) })
	t.Run("update saves change result", func(t *testing.T) { testUpdate(t, newStore(t)) })
	t.Run("update error saves nothing", func(t *testing.T) { testUpdateError(t, newStore(t)) })
	t.Run("concurrent updates are atomic", func(t *testing.T) { testConcurrentUpdates(t, newStore(t)) })
	t.Run("delete removes item", func(t *testing.T) { testDelete(t, newStore(t)) })
}

func idFor(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", n))
}

func mustInsert(t *testing.T, store todo.Store, item todo.Todo) {
	t.Helper()
	if err := store.Insert(t.Context(), item); err != nil {
		t.Fatalf("Insert(%+v) error: %v", item, err)
	}
}

func testListEmpty(t *testing.T, store todo.Store) {
	items, err := store.List(t.Context())
	if err != nil || items == nil || len(items) != 0 {
		t.Errorf("List() on empty store = (%#v, %v), want empty non-nil slice", items, err)
	}
}

func testListOrder(t *testing.T, store todo.Store) {
	// IDs deliberately inserted out of lexical order: ordering must follow insertion, not ID.
	for _, n := range []int{3, 1, 2} {
		mustInsert(t, store, todo.Todo{ID: idFor(n), Title: fmt.Sprintf("item %d", n)})
	}
	items, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	for i, n := range []int{3, 1, 2} {
		if i >= len(items) || items[i].ID != idFor(n) {
			t.Fatalf("List() = %+v, want IDs in insertion order 3,1,2", items)
		}
	}
}

func testGet(t *testing.T, store todo.Store) {
	want := todo.Todo{ID: idFor(1), Title: "read", Completed: true}
	mustInsert(t, store, want)
	got, err := store.Get(t.Context(), want.ID)
	if err != nil || got != want {
		t.Errorf("Get() = (%+v, %v), want (%+v, nil)", got, err, want)
	}
}

func testNotFound(t *testing.T, store todo.Store) {
	unknown := idFor(404)
	_, getErr := store.Get(t.Context(), unknown)
	_, updateErr := store.Update(t.Context(), unknown, func(c todo.Todo) (todo.Todo, error) { return c, nil })
	deleteErr := store.Delete(t.Context(), unknown)
	for name, err := range map[string]error{"Get": getErr, "Update": updateErr, "Delete": deleteErr} {
		if !errors.Is(err, todo.ErrNotFound) {
			t.Errorf("%s(unknown) error = %v, want todo.ErrNotFound", name, err)
		}
	}
}

func testUpdate(t *testing.T, store todo.Store) {
	mustInsert(t, store, todo.Todo{ID: idFor(1), Title: "before"})
	updated, err := store.Update(t.Context(), idFor(1), func(c todo.Todo) (todo.Todo, error) {
		c.Title, c.Completed = "after", true
		return c, nil
	})
	want := todo.Todo{ID: idFor(1), Title: "after", Completed: true}
	if err != nil || updated != want {
		t.Fatalf("Update() = (%+v, %v), want (%+v, nil)", updated, err, want)
	}
	if stored, _ := store.Get(t.Context(), idFor(1)); stored != want {
		t.Errorf("Get() after Update = %+v, want %+v", stored, want)
	}
}

func testUpdateError(t *testing.T, store todo.Store) {
	original := todo.Todo{ID: idFor(1), Title: "keep"}
	mustInsert(t, store, original)
	rejected := errors.New("rejected by change")
	_, err := store.Update(t.Context(), idFor(1), func(todo.Todo) (todo.Todo, error) {
		return todo.Todo{}, rejected
	})
	if !errors.Is(err, rejected) {
		t.Errorf("Update() error = %v, want the change's error", err)
	}
	if stored, _ := store.Get(t.Context(), idFor(1)); stored != original {
		t.Errorf("Get() after failed Update = %+v, want unchanged %+v", stored, original)
	}
}

// testConcurrentUpdates detects lost updates: each goroutine appends one char via read-modify-write.
func testConcurrentUpdates(t *testing.T, store todo.Store) {
	const writers = 20
	mustInsert(t, store, todo.Todo{ID: idFor(1), Title: "x"})
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			_, err := store.Update(t.Context(), idFor(1), func(c todo.Todo) (todo.Todo, error) {
				c.Title += "x"
				return c, nil
			})
			if err != nil {
				t.Errorf("concurrent Update() error: %v", err)
			}
		})
	}
	wg.Wait()
	if stored, _ := store.Get(t.Context(), idFor(1)); len(stored.Title) != writers+1 {
		t.Errorf("title length = %d after %d concurrent appends, want %d (lost update)", len(stored.Title), writers, writers+1)
	}
}

func testDelete(t *testing.T, store todo.Store) {
	mustInsert(t, store, todo.Todo{ID: idFor(1), Title: "gone"})
	if err := store.Delete(t.Context(), idFor(1)); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if _, err := store.Get(t.Context(), idFor(1)); !errors.Is(err, todo.ErrNotFound) {
		t.Errorf("Get() after Delete error = %v, want ErrNotFound", err)
	}
}
