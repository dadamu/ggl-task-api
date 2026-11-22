package handler_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
	"github.com/dadamu/ggl-task-api/internal/infrastructures/persistence/memory"
	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
	"github.com/dadamu/ggl-task-api/internal/transport/http/dto"
)

// --- GET /tasks ---

func TestListEmpty(t *testing.T) {
	rec := do(t, newServer(t), http.MethodGet, "/tasks", "")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"data":[],"next_cursor":null}`, rec.Body.String())
}

func TestListOrderedByID(t *testing.T) {
	h := newServer(t)
	create(t, h, `{"name":"a"}`)
	create(t, h, `{"name":"b","status":1}`)

	rec := do(t, h, http.MethodGet, "/tasks", "")

	require.Equal(t, http.StatusOK, rec.Code)
	list := decode[dto.TaskList](t, rec)
	require.Len(t, list.Data, 2)
	require.Equal(t, []string{"a", "b"}, []string{list.Data[0].Name, list.Data[1].Name})
}

func TestListPagination(t *testing.T) {
	h := newServer(t)
	for i := range 25 {
		create(t, h, `{"name":"t`+strconv.Itoa(i)+`"}`)
	}
	pageIDs := func(l dto.TaskList) []int64 {
		out := make([]int64, len(l.Data))
		for i, tk := range l.Data {
			out[i] = tk.ID
		}
		return out
	}
	idRange := func(from, to int64) []int64 {
		var out []int64
		for id := from; id <= to; id++ {
			out = append(out, id)
		}
		return out
	}
	cursor := func(id int64) *int64 { return &id }

	tests := []struct {
		name     string
		query    string
		wantIDs  []int64
		wantNext *int64
	}{
		{"default page size is 20", "", idRange(1, 20), cursor(20)},
		{"explicit limit", "?limit=5", idRange(1, 5), cursor(5)},
		{"after cursor", "?limit=5&after=5", idRange(6, 10), cursor(10)},
		{"last page", "?after=20", idRange(21, 25), nil},
		{"max limit returns everything here", "?limit=100", idRange(1, 25), nil},
		{"after the last task", "?after=25", []int64{}, nil},
		{"unknown query parameters are ignored", "?limit=2&foo=bar", idRange(1, 2), cursor(2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/tasks"+tt.query, "")

			require.Equal(t, http.StatusOK, rec.Code)
			list := decode[dto.TaskList](t, rec)
			require.Equal(t, tt.wantIDs, pageIDs(list))
			require.Equal(t, tt.wantNext, list.NextCursor)
		})
	}
}

func TestListFollowingNextCursorVisitsEveryTaskOnce(t *testing.T) {
	h := newServer(t)
	for i := range 23 {
		create(t, h, `{"name":"t`+strconv.Itoa(i)+`"}`)
	}

	var seen []int64
	query := "/tasks?limit=7"
	for pages := 0; ; pages++ {
		require.Less(t, pages, 10, "pagination must terminate")
		list := decode[dto.TaskList](t, do(t, h, http.MethodGet, query, ""))
		for _, tk := range list.Data {
			seen = append(seen, tk.ID)
		}
		if list.NextCursor == nil {
			break
		}
		query = "/tasks?limit=7&after=" + strconv.FormatInt(*list.NextCursor, 10)
	}

	require.Len(t, seen, 23)
	require.IsIncreasing(t, seen)
}

func TestListInvalidQuery(t *testing.T) {
	limitReason := apierror.FieldDetail{Field: "limit", Reason: "must be an integer between 1 and 100"}
	afterReason := apierror.FieldDetail{Field: "after", Reason: "must be a non-negative integer"}

	tests := []struct {
		query       string
		wantDetails []apierror.FieldDetail
	}{
		{"?limit=0", []apierror.FieldDetail{limitReason}},
		{"?limit=101", []apierror.FieldDetail{limitReason}},
		{"?limit=-1", []apierror.FieldDetail{limitReason}},
		{"?limit=abc", []apierror.FieldDetail{limitReason}},
		{"?limit=", []apierror.FieldDetail{limitReason}},
		{"?after=-1", []apierror.FieldDetail{afterReason}},
		{"?after=abc", []apierror.FieldDetail{afterReason}},
		{"?after=99999999999999999999", []apierror.FieldDetail{afterReason}},
		{"?limit=0&after=x", []apierror.FieldDetail{limitReason, afterReason}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			rec := do(t, newServer(t), http.MethodGet, "/tasks"+tt.query, "")

			body := requireError(t, rec, http.StatusBadRequest, apierror.CodeValidationFailed)
			require.Equal(t, tt.wantDetails, body.Details)
		})
	}
}

// --- POST /tasks ---

func TestCreate(t *testing.T) {
	rec := do(t, newServer(t), http.MethodPost, "/tasks", `{"name":"Buy milk","status":1}`)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "/tasks/1", rec.Header().Get("Location"))
	require.JSONEq(t, `{
		"id": 1,
		"name": "Buy milk",
		"status": 1,
		"version": 1,
		"created_at": "2026-11-21T08:00:00.123Z",
		"updated_at": "2026-11-21T08:00:00.123Z"
	}`, rec.Body.String())
}

func TestCreateStatusDefaultsToZero(t *testing.T) {
	got := create(t, newServer(t), `{"name":"Buy milk"}`)

	require.Equal(t, 0, got.Status)
}

func TestCreateIgnoresUnknownAndServerManagedFields(t *testing.T) {
	got := create(t, newServer(t), `{"name":"a","foo":1,"id":42,"version":9}`)

	require.Equal(t, int64(1), got.ID)
	require.Equal(t, int64(1), got.Version)
}

func TestCreateValidation(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCode    string
		wantDetails []apierror.FieldDetail
	}{
		{"empty body", ``, apierror.CodeInvalidJSON, nil},
		{"malformed json", `{"name":`, apierror.CodeInvalidJSON, nil},
		{"not an object", `[]`, apierror.CodeInvalidJSON, nil},
		{
			"missing name", `{"status":0}`, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "name", Reason: "is required"}},
		},
		{
			"null name", `{"name":null}`, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "name", Reason: "is required"}},
		},
		{
			"blank name", `{"name":"   "}`, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "name", Reason: "must not be blank"}},
		},
		{
			"name too long", `{"name":"` + strings.Repeat("a", 256) + `"}`, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "name", Reason: "must be at most 255 characters"}},
		},
		{
			"status out of range", `{"name":"a","status":2}`, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "status", Reason: "must be 0 or 1"}},
		},
		{
			"status wrong type", `{"name":"a","status":"1"}`, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "status", Reason: "must be an integer"}},
		},
		{
			"name wrong type", `{"name":1}`, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "name", Reason: "must be a string"}},
		},
		{
			"all invalid", `{"name":"","status":5}`, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{
				{Field: "name", Reason: "must not be blank"},
				{Field: "status", Reason: "must be 0 or 1"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newServer(t)

			rec := do(t, h, http.MethodPost, "/tasks", tt.body)

			body := requireError(t, rec, http.StatusBadRequest, tt.wantCode)
			require.Equal(t, tt.wantDetails, body.Details)
			require.JSONEq(t, `{"data":[],"next_cursor":null}`, do(t, h, http.MethodGet, "/tasks", "").Body.String(), "nothing stored")
		})
	}
}

// --- PUT /tasks/{id} ---

func TestUpdate(t *testing.T) {
	h := newServer(t)
	created := create(t, h, `{"name":"a"}`)

	rec := do(t, h, http.MethodPut, "/tasks/1", `{"name":"b","status":1,"version":1}`)

	require.Equal(t, http.StatusOK, rec.Code)
	got := decode[dto.Task](t, rec)
	require.Equal(t, dto.Task{
		ID:        1,
		Name:      "b",
		Status:    1,
		Version:   2,
		CreatedAt: created.CreatedAt,
		UpdatedAt: created.UpdatedAt,
	}, got)
}

func TestUpdateIgnoresBodyID(t *testing.T) {
	h := newServer(t)
	create(t, h, `{"name":"a"}`)
	create(t, h, `{"name":"b"}`)

	rec := do(t, h, http.MethodPut, "/tasks/1", `{"id":2,"name":"c","status":0,"version":1}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(1), decode[dto.Task](t, rec).ID)
}

func TestUpdateErrors(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		body        string
		wantStatus  int
		wantCode    string
		wantDetails []apierror.FieldDetail
	}{
		{
			"invalid id", "/tasks/abc", `{"name":"b","status":0,"version":1}`,
			http.StatusBadRequest, apierror.CodeInvalidID, nil,
		},
		{
			"id overflows int64", "/tasks/99999999999999999999", `{"name":"b","status":0,"version":1}`,
			http.StatusBadRequest, apierror.CodeInvalidID, nil,
		},
		{
			"zero id", "/tasks/0", `{"name":"b","status":0,"version":1}`,
			http.StatusBadRequest, apierror.CodeInvalidID, nil,
		},
		{
			"negative id", "/tasks/-1", `{"name":"b","status":0,"version":1}`,
			http.StatusBadRequest, apierror.CodeInvalidID, nil,
		},
		{
			"all fields missing", "/tasks/1", `{}`,
			http.StatusBadRequest, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{
				{Field: "name", Reason: "is required"},
				{Field: "status", Reason: "is required"},
				{Field: "version", Reason: "is required"},
			},
		},
		{
			"version missing", "/tasks/1", `{"name":"b","status":0}`,
			http.StatusBadRequest, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "version", Reason: "is required"}},
		},
		{
			"invalid status", "/tasks/1", `{"name":"b","status":3,"version":1}`,
			http.StatusBadRequest, apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "status", Reason: "must be 0 or 1"}},
		},
		{
			"not found", "/tasks/999", `{"name":"b","status":0,"version":1}`,
			http.StatusNotFound, apierror.CodeTaskNotFound, nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newServer(t)
			before := create(t, h, `{"name":"a"}`)

			rec := do(t, h, http.MethodPut, tt.path, tt.body)

			body := requireError(t, rec, tt.wantStatus, tt.wantCode)
			require.Equal(t, tt.wantDetails, body.Details)
			list := decode[dto.TaskList](t, do(t, h, http.MethodGet, "/tasks", ""))
			require.Equal(t, []dto.Task{before}, list.Data, "failed update must not change state")
		})
	}
}

func TestUpdateVersionConflictReturnsCurrent(t *testing.T) {
	h := newServer(t)
	create(t, h, `{"name":"a"}`)
	first := do(t, h, http.MethodPut, "/tasks/1", `{"name":"b","status":0,"version":1}`)
	require.Equal(t, http.StatusOK, first.Code)
	current := decode[dto.Task](t, first)

	rec := do(t, h, http.MethodPut, "/tasks/1", `{"name":"c","status":1,"version":1}`)

	body := requireError(t, rec, http.StatusConflict, apierror.CodeVersionConflict)
	require.NotNil(t, body.Current)
	require.Equal(t, current, *body.Current)
}

// --- DELETE /tasks/{id} ---

func TestDelete(t *testing.T) {
	h := newServer(t)
	create(t, h, `{"name":"a"}`)

	rec := do(t, h, http.MethodDelete, "/tasks/1", "")

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
	requireError(t, do(t, h, http.MethodDelete, "/tasks/1", ""), http.StatusNotFound, apierror.CodeTaskNotFound)
	require.JSONEq(t, `{"data":[],"next_cursor":null}`, do(t, h, http.MethodGet, "/tasks", "").Body.String())
}

func TestDeleteInvalidID(t *testing.T) {
	requireError(t, do(t, newServer(t), http.MethodDelete, "/tasks/x", ""), http.StatusBadRequest, apierror.CodeInvalidID)
}

// --- Unexpected errors ---

type failingListRepo struct{ task.Repository }

func (failingListRepo) List(context.Context, task.ListQuery) ([]task.Task, error) {
	return nil, errors.New("secret internal detail")
}

func TestUnexpectedErrorIs500WithoutDetails(t *testing.T) {
	h := newServerWithRepo(t, failingListRepo{memory.NewTaskRepository()})

	rec := do(t, h, http.MethodGet, "/tasks", "")

	requireError(t, rec, http.StatusInternalServerError, apierror.CodeInternal)
	require.NotContains(t, rec.Body.String(), "secret")
}
