// Package pgstore is the PostgreSQL todo.Store the service runs on in every environment
// (README "Design decisions" §1). pgx stays inside this package: the rest of the code only
// sees the todo.Store interface, so the driver can be swapped without touching the domain.
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/die6ojunges/todo-service/internal/todo"
)

const (
	selectTodos     = "SELECT id, title, completed FROM todos"
	selectTodoByID  = selectTodos + " WHERE id = $1"
	insertTodo      = "INSERT INTO todos (id, title, completed) VALUES ($1, $2, $3)"
	updateTodoByID  = "UPDATE todos SET title = $2, completed = $3 WHERE id = $1"
	deleteTodoByID  = "DELETE FROM todos WHERE id = $1"
	lockTodoForEdit = selectTodoByID + " FOR UPDATE"
)

// Store persists items in the todos table created by Migrate.
type Store struct {
	pool *pgxpool.Pool
}

// rowQuerier is satisfied by both the pool and a transaction, so reads share one code path.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// New returns a store backed by pool; the caller owns the pool and closes it on shutdown.
//
//	svc := todo.NewService(pgstore.New(pool), uuid.New)
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// List returns every item ordered by creation (the seq column), never nil.
func (s *Store) List(ctx context.Context) ([]todo.Todo, error) {
	rows, err := s.pool.Query(ctx, selectTodos+" ORDER BY seq")
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}
	items, err := pgx.CollectRows(rows, scanTodo)
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}
	return items, nil
}

// Get returns the item with id or todo.ErrNotFound.
func (s *Store) Get(ctx context.Context, id uuid.UUID) (todo.Todo, error) {
	return queryTodo(ctx, s.pool, selectTodoByID, id)
}

// Insert adds a new item; a duplicate ID is a programming error and fails on the primary key.
func (s *Store) Insert(ctx context.Context, item todo.Todo) error {
	if _, err := s.pool.Exec(ctx, insertTodo, item.ID, item.Title, item.Completed); err != nil {
		return fmt.Errorf("insert todo id %s (want a new unique id): %w", item.ID, err)
	}
	return nil
}

// Update locks the row with SELECT … FOR UPDATE for the whole read-change-write, so concurrent
// updates of the same item (on any replica) are serialised instead of overwriting each other.
func (s *Store) Update(ctx context.Context, id uuid.UUID, change func(todo.Todo) (todo.Todo, error)) (todo.Todo, error) {
	var updated todo.Todo
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := queryTodo(ctx, tx, lockTodoForEdit, id)
		if err != nil {
			return err
		}
		if updated, err = change(current); err != nil {
			return err
		}
		updated.ID = id // the ID is immutable whatever change returns
		return execOnTodo(ctx, tx, updateTodoByID, id, updated.Title, updated.Completed)
	})
	if err != nil {
		return todo.Todo{}, err
	}
	return updated, nil
}

// Delete removes the item with id or returns todo.ErrNotFound.
func (s *Store) Delete(ctx context.Context, id uuid.UUID) error {
	return execOnTodo(ctx, s.pool, deleteTodoByID, id)
}

// todoExecer is satisfied by both the pool and a transaction.
type todoExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// execOnTodo runs a statement that must touch exactly the row with id (passed as $1).
func execOnTodo(ctx context.Context, db todoExecer, sql string, id uuid.UUID, args ...any) error {
	tag, err := db.Exec(ctx, sql, append([]any{id}, args...)...)
	if err != nil {
		return fmt.Errorf("todo id %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return notFound(id)
	}
	return nil
}

func queryTodo(ctx context.Context, db rowQuerier, sql string, id uuid.UUID) (todo.Todo, error) {
	var item todo.Todo
	err := db.QueryRow(ctx, sql, id).Scan(&item.ID, &item.Title, &item.Completed)
	if errors.Is(err, pgx.ErrNoRows) {
		return todo.Todo{}, notFound(id)
	}
	if err != nil {
		return todo.Todo{}, fmt.Errorf("get todo id %s: %w", id, err)
	}
	return item, nil
}

func scanTodo(row pgx.CollectableRow) (todo.Todo, error) {
	var item todo.Todo
	err := row.Scan(&item.ID, &item.Title, &item.Completed)
	return item, err
}

func notFound(id uuid.UUID) error {
	return fmt.Errorf("%w: id %s", todo.ErrNotFound, id)
}
