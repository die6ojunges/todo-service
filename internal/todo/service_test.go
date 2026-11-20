package todo_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/die6ojunges/todo-service/internal/todo"
	"github.com/die6ojunges/todo-service/internal/todo/todotest"
)

// sequentialIDs is a deterministic IDGenerator so assertions can name exact IDs.
func sequentialIDs() todo.IDGenerator {
	next := 0
	return func() uuid.UUID {
		next++
		return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", next))
	}
}

func newTestService() *todo.Service {
	return todo.NewService(todotest.NewFakeStore(), sequentialIDs())
}

func ptr[T any](v T) *T { return &v }

func TestCreateAssignsServerIDAndTrimsTitle(t *testing.T) {
	svc := newTestService()
	created, err := svc.Create(t.Context(), todo.Draft{Title: "  buy milk  ", Completed: true})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	want := todo.Todo{ID: uuid.MustParse("00000000-0000-4000-8000-000000000001"), Title: "buy milk", Completed: true}
	if created != want {
		t.Errorf("Create() = %+v, want %+v", created, want)
	}
}

func TestCreateRejectsInvalidTitles(t *testing.T) {
	cases := map[string]string{
		"empty":      "",
		"blank":      "   ",
		"too long":   strings.Repeat("a", todo.MaxTitleLength+1),
		"multi-byte": strings.Repeat("é", todo.MaxTitleLength+1),
	}
	for name, title := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newTestService().Create(t.Context(), todo.Draft{Title: title})
			var invalid *todo.ValidationError
			if !errors.As(err, &invalid) || invalid.Field != "title" {
				t.Fatalf("Create(%q) error = %v, want *ValidationError on field title", title, err)
			}
		})
	}
}

func TestTitleLimitCountsCharactersNotBytes(t *testing.T) {
	_, err := newTestService().Create(t.Context(), todo.Draft{Title: strings.Repeat("é", todo.MaxTitleLength)})
	if err != nil {
		t.Fatalf("Create() with %d multi-byte chars error = %v, want nil", todo.MaxTitleLength, err)
	}
}

func TestListReturnsItemsInCreationOrder(t *testing.T) {
	svc := newTestService()
	for _, title := range []string{"first", "second", "third"} {
		if _, err := svc.Create(t.Context(), todo.Draft{Title: title}); err != nil {
			t.Fatalf("Create(%q) error: %v", title, err)
		}
	}
	items, err := svc.List(t.Context())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Title)
	}
	if strings.Join(got, ",") != "first,second,third" {
		t.Errorf("List() titles = %v, want [first second third]", got)
	}
}

func TestListOnEmptyStoreReturnsEmptyNotNil(t *testing.T) {
	items, err := newTestService().List(t.Context())
	if err != nil || items == nil || len(items) != 0 {
		t.Errorf("List() = (%#v, %v), want empty non-nil slice and nil error", items, err)
	}
}

func TestReplaceOverwritesEveryField(t *testing.T) {
	svc := newTestService()
	created, _ := svc.Create(t.Context(), todo.Draft{Title: "old", Completed: true})
	replaced, err := svc.Replace(t.Context(), created.ID, todo.Draft{Title: "new", Completed: false})
	if err != nil {
		t.Fatalf("Replace() error: %v", err)
	}
	want := todo.Todo{ID: created.ID, Title: "new", Completed: false}
	if replaced != want {
		t.Errorf("Replace() = %+v, want %+v", replaced, want)
	}
}

func TestPatchChangesOnlyProvidedFields(t *testing.T) {
	svc := newTestService()
	created, _ := svc.Create(t.Context(), todo.Draft{Title: "keep me"})
	patched, err := svc.Patch(t.Context(), created.ID, todo.Patch{Completed: ptr(true)})
	if err != nil {
		t.Fatalf("Patch() error: %v", err)
	}
	want := todo.Todo{ID: created.ID, Title: "keep me", Completed: true}
	if patched != want {
		t.Errorf("Patch() = %+v, want %+v", patched, want)
	}
}

func TestPatchValidatesTitleWithoutSaving(t *testing.T) {
	svc := newTestService()
	created, _ := svc.Create(t.Context(), todo.Draft{Title: "original"})
	_, err := svc.Patch(t.Context(), created.ID, todo.Patch{Title: ptr("  ")})
	var invalid *todo.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("Patch() error = %v, want *ValidationError", err)
	}
	stored, _ := svc.Get(t.Context(), created.ID)
	if stored.Title != "original" {
		t.Errorf("stored title = %q after rejected patch, want %q", stored.Title, "original")
	}
}

func TestOperationsOnUnknownIDReturnNotFound(t *testing.T) {
	svc := newTestService()
	unknown := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	checks := map[string]error{
		"Get": func() error { _, err := svc.Get(t.Context(), unknown); return err }(),
		"Replace": func() error {
			_, err := svc.Replace(t.Context(), unknown, todo.Draft{Title: "x"})
			return err
		}(),
		"Patch":  func() error { _, err := svc.Patch(t.Context(), unknown, todo.Patch{}); return err }(),
		"Delete": svc.Delete(t.Context(), unknown),
	}
	for name, err := range checks {
		if !errors.Is(err, todo.ErrNotFound) {
			t.Errorf("%s(unknown id) error = %v, want ErrNotFound", name, err)
		}
	}
}

func TestDeleteRemovesItem(t *testing.T) {
	svc := newTestService()
	created, _ := svc.Create(t.Context(), todo.Draft{Title: "temp"})
	if err := svc.Delete(t.Context(), created.ID); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if _, err := svc.Get(t.Context(), created.ID); !errors.Is(err, todo.ErrNotFound) {
		t.Errorf("Get() after Delete error = %v, want ErrNotFound", err)
	}
}
