package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/die6ojunges/todo-service/internal/httpapi"
	"github.com/die6ojunges/todo-service/internal/todo"
	"github.com/die6ojunges/todo-service/internal/todo/todotest"
)

// apiClient drives the full router in-process, like a real HTTP client would.
type apiClient struct {
	t      *testing.T
	router http.Handler
}

func newAPIClient(t *testing.T) *apiClient {
	t.Helper()
	next := 0
	ids := func() uuid.UUID {
		next++
		return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", next))
	}
	service := todo.NewService(todotest.NewFakeStore(), ids)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	router := httpapi.NewRouter(httpapi.NewHealthChecks(pingOK{}), httpapi.NewTodoAPI(service, logger), logger)
	return &apiClient{t: t, router: router}
}

func (c *apiClient) do(method, path, contentType, body string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequestWithContext(c.t.Context(), method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)
	return rec
}

func (c *apiClient) json(method, path, body string) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.do(method, path, "application/json", body)
}

func (c *apiClient) create(title string) {
	c.t.Helper()
	if rec := c.json(http.MethodPost, "/todos", fmt.Sprintf(`{"title":%q}`, title)); rec.Code != http.StatusCreated {
		c.t.Fatalf("seed POST /todos = %d, body %s", rec.Code, rec.Body)
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, want, rec.Body)
	}
}

func assertJSONBody(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got, expected any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not JSON: %v; body: %s", err, rec.Body)
	}
	_ = json.Unmarshal([]byte(want), &expected)
	if fmt.Sprint(got) != fmt.Sprint(expected) {
		t.Errorf("body = %s, want %s", rec.Body, want)
	}
}

// assertProblem checks an RFC 9457 error response.
func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	assertStatus(t, rec, wantStatus)
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var problem struct {
		Status int    `json:"status"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Status != wantStatus || problem.Title == "" {
		t.Errorf("problem body = %s, want status %d and a title", rec.Body, wantStatus)
	}
}

// pingOK is a named fake readiness dependency that is always up.
type pingOK struct{}

func (pingOK) Ping(context.Context) error { return nil }
