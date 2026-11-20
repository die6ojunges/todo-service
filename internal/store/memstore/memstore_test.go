package memstore_test

import (
	"testing"

	"github.com/die6ojunges/todo-service/internal/store/memstore"
	"github.com/die6ojunges/todo-service/internal/todo"
	"github.com/die6ojunges/todo-service/internal/todo/todotest"
)

func TestMemStoreSatisfiesStoreContract(t *testing.T) {
	todotest.RunStoreContract(t, func(*testing.T) todo.Store { return memstore.New() })
}
