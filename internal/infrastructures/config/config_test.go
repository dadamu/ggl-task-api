package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/infrastructures/config"
)

// unsetenv removes key for the duration of the test and restores it afterwards.
func unsetenv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "") // registers restoration of the original value
	require.NoError(t, os.Unsetenv(key))
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string // variables not listed are unset
		want config.Config
	}{
		{
			name: "defaults when unset",
			want: config.Config{Port: "8080", GinMode: "debug"},
		},
		{
			name: "defaults when empty",
			env:  map[string]string{"PORT": "", "GIN_MODE": ""},
			want: config.Config{Port: "8080", GinMode: "debug"},
		},
		{
			name: "overrides",
			env:  map[string]string{"PORT": "9090", "GIN_MODE": "release"},
			want: config.Config{Port: "9090", GinMode: "release"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetenv(t, "PORT")
			unsetenv(t, "GIN_MODE")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			require.Equal(t, tt.want, config.Load())
		})
	}
}
