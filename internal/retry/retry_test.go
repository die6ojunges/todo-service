package retry

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// flakyAttempt fails failures times, then succeeds; it counts every call.
type flakyAttempt struct {
	failures int
	calls    int
}

func (f *flakyAttempt) run(context.Context) error {
	f.calls++
	if f.calls <= f.failures {
		return errors.New("database not up yet")
	}
	return nil
}

// failureLog records the waits Until announces, so tests check the backoff without sleeping long.
type failureLog struct{ waits []time.Duration }

func (l *failureLog) record(_ error, wait time.Duration) { l.waits = append(l.waits, wait) }

func TestUntilRetriesWithDoublingCappedBackoff(t *testing.T) {
	attempt, log := &flakyAttempt{failures: 4}, &failureLog{}
	backoff := Backoff{Initial: time.Millisecond, Max: 4 * time.Millisecond}
	if err := Until(t.Context(), backoff, attempt.run, log.record); err != nil {
		t.Fatalf("Until() error = %v, want nil after the attempt succeeds", err)
	}
	want := []time.Duration{1, 2, 4, 4}
	for i := range want {
		want[i] *= time.Millisecond
	}
	if attempt.calls != 5 || !slices.Equal(log.waits, want) {
		t.Errorf("calls = %d, waits = %v; want 5 calls and waits %v", attempt.calls, log.waits, want)
	}
}

func TestUntilStopsWhenContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	attempt := &flakyAttempt{failures: 1_000}
	err := Until(ctx, Backoff{Initial: time.Hour, Max: time.Hour}, attempt.run, (&failureLog{}).record)
	if !errors.Is(err, context.Canceled) || attempt.calls != 1 {
		t.Errorf("Until() = %v after %d calls, want context.Canceled after 1 call", err, attempt.calls)
	}
}
