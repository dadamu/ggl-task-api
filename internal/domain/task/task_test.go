package task_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
)

var now = time.Date(2026, 11, 21, 8, 0, 0, 123456789, time.FixedZone("UTC+8", 8*60*60))

func TestNew(t *testing.T) {
	got, err := task.New("Buy milk", task.StatusIncomplete, now)
	require.NoError(t, err)

	// UTC with millisecond precision.
	want := time.Date(2026, 11, 21, 0, 0, 0, 123000000, time.UTC)
	require.Equal(t, task.Task{
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
				require.Equal(t, tt.inName, got.Name)
				return
			}
			var verr *task.ValidationError
			require.ErrorAs(t, err, &verr)
			require.Equal(t, tt.wantFields, verr.Fields)
		})
	}
}

func TestUpdate(t *testing.T) {
	orig, err := task.New("Buy milk", task.StatusIncomplete, now)
	require.NoError(t, err)

	later := now.Add(time.Hour)
	updated := orig
	require.NoError(t, updated.Update("Buy oat milk", task.StatusCompleted, later))

	require.Equal(t, task.Task{
		Name:      "Buy oat milk",
		Status:    task.StatusCompleted,
		Version:   orig.Version,   // bumped by the repository, not the entity
		CreatedAt: orig.CreatedAt, // preserved
		UpdatedAt: later.UTC().Truncate(time.Millisecond),
	}, updated)
}

func TestUpdateInvalidLeavesTaskUnchanged(t *testing.T) {
	orig, err := task.New("Buy milk", task.StatusIncomplete, now)
	require.NoError(t, err)

	updated := orig
	err = updated.Update(" ", 5, now.Add(time.Hour))

	var verr *task.ValidationError
	require.ErrorAs(t, err, &verr)
	require.Len(t, verr.Fields, 2)
	require.Equal(t, orig, updated)
}
