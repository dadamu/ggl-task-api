package task_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
)

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
		t.Run(strconv.Itoa(int(tt.status)), func(t *testing.T) {
			require.Equal(t, tt.want, tt.status.Valid())
		})
	}
}
