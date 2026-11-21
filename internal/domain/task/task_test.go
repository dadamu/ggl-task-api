package task_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
)

var now = time.Date(2026, 11, 21, 8, 0, 0, 123456789, time.FixedZone("UTC+8", 8*60*60))

func TestStatusValid(t *testing.T) {
	tests := []struct {
		status task.Status
		want   bool
	}{
		{-1, false},
		{task.StatusIncomplete, true},
		{task.StatusCompleted, true},
		{2, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.status.Valid(), "status %d", tt.status)
	}
}

func TestNew(t *testing.T) {
	got, err := task.New("Buy milk", task.StatusIncomplete, now)
	require.NoError(t, err)

	want := time.Date(2026, 11, 21, 0, 0, 0, 123000000, time.UTC)
	assert.Equal(t, task.Task{
		Name:      "Buy milk",
		Status:    task.StatusIncomplete,
		Version:   task.InitialVersion,
		CreatedAt: want,
		UpdatedAt: want,
	}, got)
}

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name       string
		inName     string
		inStatus   task.Status
		wantFields []task.FieldError
	}{
		{"valid ascii max length", strings.Repeat("a", 255), 0, nil},
		{"valid multibyte max length", strings.Repeat("中", 255), 1, nil},
		{"name keeps surrounding spaces", "  Buy milk  ", 0, nil},
		{"empty name", "", 0, []task.FieldError{{Field: "name", Reason: "must not be blank"}}},
		{"whitespace-only name", " \t\n ", 0, []task.FieldError{{Field: "name", Reason: "must not be blank"}}},
		{"name too long", strings.Repeat("a", 256), 0, []task.FieldError{{Field: "name", Reason: "must be at most 255 characters"}}},
		{"multibyte name too long", strings.Repeat("中", 256), 0, []task.FieldError{{Field: "name", Reason: "must be at most 255 characters"}}},
		{"status negative", "x", -1, []task.FieldError{{Field: "status", Reason: "must be 0 or 1"}}},
		{"status too large", "x", 2, []task.FieldError{{Field: "status", Reason: "must be 0 or 1"}}},
		{"all fields invalid", "", 2, []task.FieldError{
			{Field: "name", Reason: "must not be blank"},
			{Field: "status", Reason: "must be 0 or 1"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := task.New(tt.inName, tt.inStatus, now)
			if tt.wantFields == nil {
				require.NoError(t, err)
				assert.Equal(t, tt.inName, got.Name)
				return
			}
			var verr *task.ValidationError
			require.ErrorAs(t, err, &verr)
			assert.Equal(t, tt.wantFields, verr.Fields)
		})
	}
}

func TestUpdate(t *testing.T) {
	orig, err := task.New("Buy milk", task.StatusIncomplete, now)
	require.NoError(t, err)

	later := now.Add(time.Hour)
	updated := orig
	require.NoError(t, updated.Update("Buy oat milk", task.StatusCompleted, later))

	assert.Equal(t, "Buy oat milk", updated.Name)
	assert.Equal(t, task.StatusCompleted, updated.Status)
	assert.Equal(t, orig.CreatedAt, updated.CreatedAt, "created_at must be preserved")
	assert.Equal(t, later.UTC().Truncate(time.Millisecond), updated.UpdatedAt)
	assert.Equal(t, orig.Version, updated.Version, "version is bumped by the repository, not the entity")
}

func TestUpdateInvalidLeavesTaskUnchanged(t *testing.T) {
	orig, err := task.New("Buy milk", task.StatusIncomplete, now)
	require.NoError(t, err)

	updated := orig
	err = updated.Update(" ", 5, now.Add(time.Hour))

	var verr *task.ValidationError
	require.ErrorAs(t, err, &verr)
	assert.Len(t, verr.Fields, 2)
	assert.Equal(t, orig, updated)
}

func TestVersionConflictError(t *testing.T) {
	current := task.Task{ID: 1, Version: 3}
	var err error = &task.VersionConflictError{Current: current}

	require.ErrorIs(t, err, task.ErrVersionConflict)
	require.NotErrorIs(t, err, task.ErrNotFound)
	require.EqualError(t, err, task.ErrVersionConflict.Error())

	var cerr *task.VersionConflictError
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, current, cerr.Current)

	wrapped := errors.Join(errors.New("context"), err)
	require.ErrorIs(t, wrapped, task.ErrVersionConflict)
}

func TestValidationErrorMessage(t *testing.T) {
	err := &task.ValidationError{Fields: []task.FieldError{
		{Field: "name", Reason: "must not be blank"},
		{Field: "status", Reason: "must be 0 or 1"},
	}}
	assert.Equal(t, "invalid task: name: must not be blank; status: must be 0 or 1", err.Error())
}
