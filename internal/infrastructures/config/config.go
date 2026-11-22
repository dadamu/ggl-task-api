// Package config loads application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds runtime configuration.
type Config struct {
	Port           string
	GinMode        string
	SwaggerEnabled bool
	// PprofEnabled starts a separate net/http/pprof server on PprofAddr.
	// Off by default; never expose it publicly.
	PprofEnabled bool
	PprofAddr    string
}

// Load reads configuration from environment variables with defaults.
// Unset or empty variables use the default; malformed values are an error so
// that typos fail fast instead of being silently ignored.
func Load() (Config, error) {
	swagger, err := getBool("SWAGGER_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	pprof, err := getBool("PPROF_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Port:           getEnv("PORT", "8080"),
		GinMode:        getEnv("GIN_MODE", "debug"),
		SwaggerEnabled: swagger,
		PprofEnabled:   pprof,
		PprofAddr:      getEnv("PPROF_ADDR", "localhost:6060"),
	}, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getBool(key string, fallback bool) (bool, error) {
	v := getEnv(key, "")
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("config: %s must be a boolean, got %q", key, v)
	}
	return b, nil
}
