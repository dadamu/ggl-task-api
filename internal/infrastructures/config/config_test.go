package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/infrastructures/config"
)

var keys = []string{"PORT", "GIN_MODE", "SWAGGER_ENABLED", "PPROF_ENABLED", "PPROF_ADDR"}

// unsetenv removes key for the duration of the test and restores it afterwards.
func unsetenv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "") // registers restoration of the original value
	require.NoError(t, os.Unsetenv(key))
}

func TestLoad(t *testing.T) {
	defaults := config.Config{Port: "8080", GinMode: "debug", SwaggerEnabled: true, PprofAddr: "localhost:6060"}

	tests := []struct {
		name string
		env  map[string]string // variables not listed are unset
		want config.Config
	}{
		{name: "defaults when unset", want: defaults},
		{
			name: "defaults when empty",
			env:  map[string]string{"PORT": "", "GIN_MODE": "", "SWAGGER_ENABLED": "", "PPROF_ENABLED": "", "PPROF_ADDR": ""},
			want: defaults,
		},
		{
			name: "overrides",
			env: map[string]string{
				"PORT": "9090", "GIN_MODE": "release", "SWAGGER_ENABLED": "false",
				"PPROF_ENABLED": "true", "PPROF_ADDR": ":6060",
			},
			want: config.Config{Port: "9090", GinMode: "release", SwaggerEnabled: false, PprofEnabled: true, PprofAddr: ":6060"},
		},
		{
			name: "boolean forms accepted by strconv.ParseBool",
			env:  map[string]string{"SWAGGER_ENABLED": "0"},
			want: config.Config{Port: "8080", GinMode: "debug", SwaggerEnabled: false, PprofAddr: "localhost:6060"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range keys {
				unsetenv(t, k)
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := config.Load()

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestLoadRejectsMalformedBoolean(t *testing.T) {
	for _, key := range []string{"SWAGGER_ENABLED", "PPROF_ENABLED"} {
		t.Run(key, func(t *testing.T) {
			for _, k := range keys {
				unsetenv(t, k)
			}
			t.Setenv(key, "yes")

			_, err := config.Load()

			require.ErrorContains(t, err, key)
		})
	}
}
