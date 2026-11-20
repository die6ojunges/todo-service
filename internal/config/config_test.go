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

// requiredOnly sets just the variables without a default.
var requiredOnly = map[string]string{"DATABASE_URL": "postgres://todo:todo@db:5432/todo"}

// withRequired returns vars plus the required variables, so each case varies only what it tests.
func withRequired(vars map[string]string) map[string]string {
	merged := map[string]string{}
	for key, value := range requiredOnly {
		merged[key] = value
	}
	for key, value := range vars {
		merged[key] = value
	}
	return merged
}

func TestLoadUsesDefaultsWhenOnlyRequiredIsSet(t *testing.T) {
	cfg, err := Load(fakeEnv(requiredOnly))
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
		"DATABASE_URL":          "postgres://u:p@host:5432/db",
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
	if cfg.DatabaseURL != "postgres://u:p@host:5432/db" {
		t.Errorf("DatabaseURL = %q, want the DATABASE_URL value", cfg.DatabaseURL)
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
		{"missing database url", map[string]string{"DATABASE_URL": ""}, "missing DATABASE_URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(fakeEnv(withRequired(tc.vars)))
			if err == nil {
				t.Fatalf("Load() error = nil, want error containing %q", tc.wantInErr)
			}
			if !strings.Contains(err.Error(), tc.wantInErr) {
				t.Errorf("Load() error = %q, want it to contain %q", err, tc.wantInErr)
			}
		})
	}
}

// Reporting every bad variable at once saves a fix-restart cycle per variable.
func TestLoadReportsAllInvalidValuesAtOnce(t *testing.T) {
	_, err := Load(fakeEnv(map[string]string{"PORT": "abc"}))
	for _, want := range []string{`PORT="abc"`, "missing DATABASE_URL"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Load() error = %v, want it to contain %q", err, want)
		}
	}
}
