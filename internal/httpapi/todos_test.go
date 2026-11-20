package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

const (
	firstID   = "00000000-0000-4000-8000-000000000001"
	unknownID = "99999999-9999-4999-8999-999999999999"
)

func TestListStartsEmpty(t *testing.T) {
	rec := newAPIClient(t).json(http.MethodGet, "/todos", "")
	assertStatus(t, rec, http.StatusOK)
	assertJSONBody(t, rec, `[]`)
}

func TestCreateReturns201WithLocation(t *testing.T) {
	rec := newAPIClient(t).json(http.MethodPost, "/todos", `{"title":"  write tests  "}`)
	assertStatus(t, rec, http.StatusCreated)
	if got := rec.Header().Get("Location"); got != "/todos/"+firstID {
		t.Errorf("Location = %q, want /todos/%s", got, firstID)
	}
	assertJSONBody(t, rec, `{"id":"`+firstID+`","title":"write tests","completed":false}`)
}

func TestListAndGetReturnCreatedItems(t *testing.T) {
	api := newAPIClient(t)
	api.create("one")
	api.create("two")
	assertJSONBody(t, api.json(http.MethodGet, "/todos", ""),
		`[{"id":"`+firstID+`","title":"one","completed":false},
		  {"id":"00000000-0000-4000-8000-000000000002","title":"two","completed":false}]`)
	assertJSONBody(t, api.json(http.MethodGet, "/todos/"+firstID, ""),
		`{"id":"`+firstID+`","title":"one","completed":false}`)
}

func TestPutReplacesWholeItem(t *testing.T) {
	api := newAPIClient(t)
	api.create("old")
	rec := api.json(http.MethodPut, "/todos/"+firstID, `{"title":"new","completed":true}`)
	assertStatus(t, rec, http.StatusOK)
	assertJSONBody(t, rec, `{"id":"`+firstID+`","title":"new","completed":true}`)
}

func TestPutRequiresEveryField(t *testing.T) {
	api := newAPIClient(t)
	api.create("old")
	assertProblem(t, api.json(http.MethodPut, "/todos/"+firstID, `{"title":"only title"}`), http.StatusUnprocessableEntity)
	assertProblem(t, api.json(http.MethodPut, "/todos/"+firstID, `{"completed":true}`), http.StatusUnprocessableEntity)
}

func TestPatchChangesOnlySentFields(t *testing.T) {
	api := newAPIClient(t)
	api.create("keep")
	for _, contentType := range []string{"application/merge-patch+json", "application/json"} {
		rec := api.do(http.MethodPatch, "/todos/"+firstID, contentType, `{"completed":true}`)
		assertStatus(t, rec, http.StatusOK)
		assertJSONBody(t, rec, `{"id":"`+firstID+`","title":"keep","completed":true}`)
	}
}

func TestPatchRejectsNullForRequiredFields(t *testing.T) {
	// In JSON Merge Patch, null means "remove the field" — impossible for required fields.
	api := newAPIClient(t)
	api.create("keep")
	assertProblem(t, api.json(http.MethodPatch, "/todos/"+firstID, `{"title":null}`), http.StatusUnprocessableEntity)
}

func TestDeleteReturns204ThenNotFound(t *testing.T) {
	api := newAPIClient(t)
	api.create("temp")
	rec := api.json(http.MethodDelete, "/todos/"+firstID, "")
	assertStatus(t, rec, http.StatusNoContent)
	if rec.Body.Len() != 0 {
		t.Errorf("DELETE body = %q, want empty", rec.Body)
	}
	assertProblem(t, api.json(http.MethodGet, "/todos/"+firstID, ""), http.StatusNotFound)
}

func TestUnknownIDReturns404(t *testing.T) {
	api := newAPIClient(t)
	assertProblem(t, api.json(http.MethodGet, "/todos/"+unknownID, ""), http.StatusNotFound)
	assertProblem(t, api.json(http.MethodPut, "/todos/"+unknownID, `{"title":"x","completed":false}`), http.StatusNotFound)
	assertProblem(t, api.json(http.MethodPatch, "/todos/"+unknownID, `{"completed":true}`), http.StatusNotFound)
	assertProblem(t, api.json(http.MethodDelete, "/todos/"+unknownID, ""), http.StatusNotFound)
}

func TestMalformedIDReturns400(t *testing.T) {
	assertProblem(t, newAPIClient(t).json(http.MethodGet, "/todos/not-a-uuid", ""), http.StatusBadRequest)
}

func TestValidationErrorsReturn422(t *testing.T) {
	api := newAPIClient(t)
	cases := map[string]string{
		"missing title": `{}`,
		"blank title":   `{"title":"   "}`,
		"long title":    `{"title":"` + strings.Repeat("a", 201) + `"}`,
		"null title":    `{"title":null}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			assertProblem(t, api.json(http.MethodPost, "/todos", body), http.StatusUnprocessableEntity)
		})
	}
}

func TestBadRequestBodiesReturn400(t *testing.T) {
	api := newAPIClient(t)
	cases := map[string]string{
		"malformed json":   `{"title":`,
		"unknown field":    `{"title":"x","priority":1}`,
		"wrong type":       `{"title":42}`,
		"trailing data":    `{"title":"x"}{"title":"y"}`,
		"client-chosen id": `{"id":"` + firstID + `","title":"x"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			assertProblem(t, api.json(http.MethodPost, "/todos", body), http.StatusBadRequest)
		})
	}
}

func TestWrongContentTypeReturns415(t *testing.T) {
	rec := newAPIClient(t).do(http.MethodPost, "/todos", "text/plain", `{"title":"x"}`)
	assertProblem(t, rec, http.StatusUnsupportedMediaType)
}

func TestOversizedBodyReturns413(t *testing.T) {
	body := `{"title":"` + strings.Repeat("a", 2<<20) + `"}`
	assertProblem(t, newAPIClient(t).json(http.MethodPost, "/todos", body), http.StatusRequestEntityTooLarge)
}

func TestHealthRoutesAreMounted(t *testing.T) {
	api := newAPIClient(t)
	assertStatus(t, api.do(http.MethodGet, "/livez", "", ""), http.StatusOK)
	assertStatus(t, api.do(http.MethodGet, "/readyz", "", ""), http.StatusOK)
}

// The detail text is part of the documented contract (README example), so pin it.
func TestReplaceWithoutCompletedReturns422WithDetail(t *testing.T) {
	api := newAPIClient(t)
	api.create("x")
	rec := api.json(http.MethodPut, "/todos/"+firstID, `{"title":"y"}`)
	assertProblem(t, rec, http.StatusUnprocessableEntity)
	want := `"detail":"invalid completed: is required for a full replacement (PUT)"`
	if !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body = %s, want it to contain %s", rec.Body.String(), want)
	}
}
