package httpapi_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errorCase is one request that must fail with an exact, fixed Problem Details body.
type errorCase struct {
	name, method, path, contentType, body string
	status                                int
	detail                                string
}

// The full client-facing error contract: every detail is a constant, never built from the request.
var errorContract = []errorCase{
	{"invalid id", "GET", "/todos/not-a-uuid", "", "", 400, "id must be a UUID"},
	{"unknown id", "GET", "/todos/" + unknownID, "", "", 404, "todo not found"},
	{"malformed json", "POST", "/todos", "application/json", `{"title":`, 400, "request body must be valid JSON"},
	{"array body", "POST", "/todos", "application/json", `[1]`, 400, "request body must be a single JSON object"},
	{"null body", "POST", "/todos", "application/json", `null`, 400, "request body must be a single JSON object"},
	{"trailing data", "POST", "/todos", "application/json", `{"title":"x"}{}`, 400, "request body must be a single JSON object"},
	{"unknown field", "POST", "/todos", "application/json", `{"title":"x","priority":1}`, 400, "request body may only contain the fields title and completed"},
	{"client-chosen id", "POST", "/todos", "application/json", `{"id":"` + firstID + `","title":"x"}`, 400, "request body may only contain the fields title and completed"},
	{"wrong type", "POST", "/todos", "application/json", `{"title":42}`, 400, "title must be a string and completed must be a boolean"},
	{"null field", "POST", "/todos", "application/json", `{"title":null}`, 422, "title and completed must not be null"},
	{"missing title", "POST", "/todos", "application/json", `{}`, 422, "title is required"},
	{"blank title", "POST", "/todos", "application/json", `{"title":"  "}`, 422, "title must not be blank"},
	{"long title", "POST", "/todos", "application/json", `{"title":"` + strings.Repeat("a", 201) + `"}`, 422, "title must be at most 200 characters"},
	{"put without completed", "PUT", "/todos/" + firstID, "application/json", `{"title":"x"}`, 422, "completed is required when replacing a todo"},
	{"wrong content type", "POST", "/todos", "text/plain", `{"title":"x"}`, 415, "content type must be application/json"},
	{"wrong patch content type", "PATCH", "/todos/" + firstID, "text/plain", `{}`, 415, "content type must be application/merge-patch+json or application/json"},
	{"oversized body", "POST", "/todos", "application/json", `{"title":"` + strings.Repeat("a", 2<<20) + `"}`, 413, "request body must be at most 1 MiB"},
}

func TestErrorResponsesUseFixedMessages(t *testing.T) {
	for _, tc := range errorContract {
		t.Run(tc.name, func(t *testing.T) {
			api := newAPIClient(t)
			api.create("existing") // so PUT/PATCH on firstID reach body validation
			rec := api.do(tc.method, tc.path, tc.contentType, tc.body)
			assertExactProblem(t, rec, tc.status, tc.detail)
		})
	}
}

// Client input must never be reflected back (injection, log forging) and internals must never
// leak (type names, parser messages), whatever part of the request carries it.
func TestErrorResponsesNeverReflectInputOrInternals(t *testing.T) {
	const marker = "zq1xMARK"
	requests := []errorCase{
		{method: "GET", path: "/todos/" + marker},
		{method: "POST", path: "/todos", contentType: "text/" + marker, body: `{}`},
		{method: "POST", path: "/todos", contentType: "application/json", body: `{"` + marker + `":1}`},
		{method: "POST", path: "/todos", contentType: "application/json", body: `{"title":"` + marker + `` + `"`},
		{method: "POST", path: "/todos", contentType: "application/json", body: `{"title":["` + marker + `"]}`},
		{method: "POST", path: "/todos", contentType: "application/json", body: `{"title":" ` + strings.Repeat(marker, 30) + `"}`},
		{method: "POST", path: "/todos", contentType: "application/json", body: `["` + marker + `"]`},
	}
	for _, req := range requests {
		body := newAPIClient(t).do(req.method, req.path, req.contentType, req.body).Body.String()
		for _, forbidden := range []string{marker, "Go value", "json:", "httpapi", "unmarshal"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s %s %s -> body %s contains %q", req.method, req.path, req.body, body, forbidden)
			}
		}
	}
}

// assertExactProblem requires exactly {title, status, detail}: no "type" or "instance", which
// would only repeat the status or reflect the request path.
func assertExactProblem(t *testing.T, rec *httptest.ResponseRecorder, status int, detail string) {
	t.Helper()
	assertStatus(t, rec, status)
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("problem body is not JSON: %v; body: %s", err, rec.Body)
	}
	want := map[string]any{"title": http.StatusText(status), "status": float64(status), "detail": detail}
	if !maps.Equal(got, want) {
		t.Errorf("problem = %v, want %v", got, want)
	}
}
