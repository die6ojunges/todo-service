// Package config loads service settings from environment variables.
// In Kubernetes these come from the ConfigMap (and Secret).
package config

import (
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
}

// Load builds a Config from the environment, applying defaults for unset keys.
//
//	cfg, err := config.Load(os.LookupEnv)
func Load(lookup LookupFunc) (Config, error) {
	port, err := portFromEnv(lookup, "PORT", 8080)
	if err != nil {
		return Config{}, err
	}
	shutdown, err := durationFromEnv(lookup, "SHUTDOWN_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	drain, err := durationFromEnv(lookup, "READINESS_DRAIN_DELAY", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	return Config{Port: port, ShutdownTimeout: shutdown, ReadinessDrainDelay: drain}, nil
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
