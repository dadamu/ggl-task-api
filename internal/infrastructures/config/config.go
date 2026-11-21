// Package config loads application configuration from environment variables.
package config

import "os"

// Config holds runtime configuration.
type Config struct {
	Port    string
	GinMode string
}

// Load reads configuration from environment variables with defaults.
func Load() Config {
	return Config{
		Port:    getEnv("PORT", "8080"),
		GinMode: getEnv("GIN_MODE", "debug"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
