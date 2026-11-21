package task_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
)

func TestVersionConflictError(t *testing.T) {
	current := task.Task{ID: 1, Version: 3}
	var err error = &task.VersionConflictError{Current: current}

	require.ErrorIs(t, err, task.ErrVersionConflict)
	require.NotErrorIs(t, err, task.ErrNotFound)
	require.EqualError(t, err, task.ErrVersionConflict.Error())

	var cerr *task.VersionConflictError
	require.ErrorAs(t, err, &cerr)
	require.Equal(t, current, cerr.Current)

	wrapped := errors.Join(errors.New("context"), err)
	require.ErrorIs(t, wrapped, task.ErrVersionConflict)
}

func TestValidationErrorMessage(t *testing.T) {
	err := &task.ValidationError{Fields: []task.FieldError{
		{Field: "name", Reason: "must not be blank"},
		{Field: "status", Reason: "must be 0 or 1"},
	}}
	require.Equal(t, "invalid task: name: must not be blank; status: must be 0 or 1", err.Error())
}
