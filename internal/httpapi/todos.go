package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/die6ojunges/todo-service/internal/todo"
)

// TodoService is what the HTTP layer needs from the domain (implemented by *todo.Service).
type TodoService interface {
	List(ctx context.Context) ([]todo.Todo, error)
	Get(ctx context.Context, id uuid.UUID) (todo.Todo, error)
	Create(ctx context.Context, draft todo.Draft) (todo.Todo, error)
	Replace(ctx context.Context, id uuid.UUID, draft todo.Draft) (todo.Todo, error)
	Patch(ctx context.Context, id uuid.UUID, patch todo.Patch) (todo.Todo, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// todoResponse is the wire format; it decouples the JSON contract from the domain type.
type todoResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

func toResponse(item todo.Todo) todoResponse {
	return todoResponse{ID: item.ID.String(), Title: item.Title, Completed: item.Completed}
}

// TodoAPI serves the /todos resource.
type TodoAPI struct {
	service TodoService
	logger  *slog.Logger
}

// NewTodoAPI builds the handlers; mount them with NewRouter.
//
//	api := httpapi.NewTodoAPI(todo.NewService(store, uuid.New), logger)
func NewTodoAPI(service TodoService, logger *slog.Logger) *TodoAPI {
	return &TodoAPI{service: service, logger: logger}
}

func (a *TodoAPI) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /todos", a.listTodos)
	mux.HandleFunc("POST /todos", a.createTodo)
	mux.HandleFunc("GET /todos/{id}", a.getTodo)
	mux.HandleFunc("PUT /todos/{id}", a.replaceTodo)
	mux.HandleFunc("PATCH /todos/{id}", a.patchTodo)
	mux.HandleFunc("DELETE /todos/{id}", a.deleteTodo)
}

func (a *TodoAPI) listTodos(w http.ResponseWriter, r *http.Request) {
	items, err := a.service.List(r.Context())
	if err != nil {
		writeError(w, r, a.logger, err)
		return
	}
	body := make([]todoResponse, 0, len(items))
	for _, item := range items {
		body = append(body, toResponse(item))
	}
	writeJSON(w, http.StatusOK, body)
}

func (a *TodoAPI) getTodo(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, r, a.logger, err)
		return
	}
	item, err := a.service.Get(r.Context(), id)
	a.respondWithItem(w, r, http.StatusOK, item, err)
}

func (a *TodoAPI) createTodo(w http.ResponseWriter, r *http.Request) {
	draft, err := decodeDraft(w, r, false)
	if err != nil {
		writeError(w, r, a.logger, err)
		return
	}
	item, err := a.service.Create(r.Context(), draft)
	if err == nil {
		w.Header().Set("Location", "/todos/"+item.ID.String())
	}
	a.respondWithItem(w, r, http.StatusCreated, item, err)
}

func (a *TodoAPI) replaceTodo(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, r, a.logger, err)
		return
	}
	draft, err := decodeDraft(w, r, true)
	if err != nil {
		writeError(w, r, a.logger, err)
		return
	}
	item, err := a.service.Replace(r.Context(), id, draft)
	a.respondWithItem(w, r, http.StatusOK, item, err)
}

func (a *TodoAPI) patchTodo(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, r, a.logger, err)
		return
	}
	patch, err := decodePatch(w, r)
	if err != nil {
		writeError(w, r, a.logger, err)
		return
	}
	item, err := a.service.Patch(r.Context(), id, patch)
	a.respondWithItem(w, r, http.StatusOK, item, err)
}

func (a *TodoAPI) deleteTodo(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err == nil {
		err = a.service.Delete(r.Context(), id)
	}
	if err != nil {
		writeError(w, r, a.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *TodoAPI) respondWithItem(w http.ResponseWriter, r *http.Request, status int, item todo.Todo, err error) {
	if err != nil {
		writeError(w, r, a.logger, err)
		return
	}
	writeJSON(w, status, toResponse(item))
}

func decodeDraft(w http.ResponseWriter, r *http.Request, requireCompleted bool) (todo.Draft, error) {
	if err := requireContentType(r, errNotJSON, "application/json"); err != nil {
		return todo.Draft{}, err
	}
	fields, err := decodeFields(w, r)
	if err != nil {
		return todo.Draft{}, err
	}
	return fields.toDraft(requireCompleted)
}

func decodePatch(w http.ResponseWriter, r *http.Request) (todo.Patch, error) {
	if err := requireContentType(r, errNotPatchJSON, "application/merge-patch+json", "application/json"); err != nil {
		return todo.Patch{}, err
	}
	fields, err := decodeFields(w, r)
	if err != nil {
		return todo.Patch{}, err
	}
	return fields.toPatch()
}
