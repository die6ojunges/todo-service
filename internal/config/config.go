// Package config loads service settings from environment variables.
// In Kubernetes these come from the ConfigMap (and Secret).
package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// LookupFunc matches os.LookupEnv; injected so tests don't depend on the process env.
type LookupFunc func(key string) (string, bool)

// Config holds every runtime setting of the service.
type Config struct {
	Port int
	// ShutdownTimeout bounds how long in-flight requests get to finish after SIGTERM.
	ShutdownTimeout time.Duration
	// ReadinessDrainDelay is how long /readyz reports 503 before the server stops,
	// giving Kubernetes time to remove the pod from the Service endpoints.
	ReadinessDrainDelay time.Duration
	// DatabaseURL is the PostgreSQL connection string; in Kubernetes it comes from a Secret.
	// Never log it: it carries the password.
	DatabaseURL string
}

// Load builds a Config from the environment, applying defaults for unset keys.
//
//	cfg, err := config.Load(os.LookupEnv)
func Load(lookup LookupFunc) (Config, error) {
	var cfg Config
	var errs [4]error
	cfg.Port, errs[0] = portFromEnv(lookup, "PORT", 8080)
	cfg.ShutdownTimeout, errs[1] = durationFromEnv(lookup, "SHUTDOWN_TIMEOUT", 15*time.Second)
	cfg.ReadinessDrainDelay, errs[2] = durationFromEnv(lookup, "READINESS_DRAIN_DELAY", 5*time.Second)
	cfg.DatabaseURL, errs[3] = requiredFromEnv(lookup, "DATABASE_URL", "a PostgreSQL URL like postgres://user:password@host:5432/db")
	if err := errors.Join(errs[:]...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// requiredFromEnv fails when key is unset or empty; the message never echoes a value,
// since required settings such as DATABASE_URL carry secrets.
func requiredFromEnv(lookup LookupFunc, key, want string) (string, error) {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return "", fmt.Errorf("missing %s: want %s", key, want)
	}
	return raw, nil
}

func portFromEnv(lookup LookupFunc, key string, fallback int) (int, error) {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid %s=%q: want an integer between 1 and 65535", key, raw)
	}
	return port, nil
}

func durationFromEnv(lookup LookupFunc, key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid %s=%q: want a non-negative Go duration like \"10s\"", key, raw)
	}
	return value, nil
}
