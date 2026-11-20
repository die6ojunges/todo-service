package config

import (
	"strings"
	"testing"
	"time"
)

// fakeEnv builds a lookup function backed by a map, so tests never touch the real process env.
func fakeEnv(vars map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		value, ok := vars[key]
		return value, ok
	}
}

func TestLoadUsesDefaultsWhenEnvIsEmpty(t *testing.T) {
	cfg, err := Load(fakeEnv(nil))
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 15s", cfg.ShutdownTimeout)
	}
	if cfg.ReadinessDrainDelay != 5*time.Second {
		t.Errorf("ReadinessDrainDelay = %v, want 5s", cfg.ReadinessDrainDelay)
	}
}

func TestLoadReadsValuesFromEnv(t *testing.T) {
	cfg, err := Load(fakeEnv(map[string]string{
		"PORT":                  "9090",
		"SHUTDOWN_TIMEOUT":      "30s",
		"READINESS_DRAIN_DELAY": "2s",
	}))
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 30s", cfg.ShutdownTimeout)
	}
	if cfg.ReadinessDrainDelay != 2*time.Second {
		t.Errorf("ReadinessDrainDelay = %v, want 2s", cfg.ReadinessDrainDelay)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name      string
		vars      map[string]string
		wantInErr string
	}{
		{"non-numeric port", map[string]string{"PORT": "abc"}, `PORT="abc"`},
		{"port out of range", map[string]string{"PORT": "70000"}, `PORT="70000"`},
		{"zero port", map[string]string{"PORT": "0"}, `PORT="0"`},
		{"bad duration", map[string]string{"SHUTDOWN_TIMEOUT": "soon"}, `SHUTDOWN_TIMEOUT="soon"`},
		{"negative duration", map[string]string{"READINESS_DRAIN_DELAY": "-1s"}, `READINESS_DRAIN_DELAY="-1s"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(fakeEnv(tc.vars))
			if err == nil {
				t.Fatalf("Load() error = nil, want error containing %q", tc.wantInErr)
			}
			if !strings.Contains(err.Error(), tc.wantInErr) {
				t.Errorf("Load() error = %q, want it to contain %q", err, tc.wantInErr)
			}
		})
	}
}
