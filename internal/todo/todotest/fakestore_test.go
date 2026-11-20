package todotest_test

import (
	"testing"

	"github.com/die6ojunges/todo-service/internal/todo"
	"github.com/die6ojunges/todo-service/internal/todo/todotest"
)

func TestFakeStoreSatisfiesStoreContract(t *testing.T) {
	todotest.RunStoreContract(t, func(*testing.T) todo.Store { return todotest.NewFakeStore() })
}
