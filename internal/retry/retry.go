// Package retry repeats an operation with exponential backoff until it succeeds or the
// context ends. The service uses it to wait for PostgreSQL on startup (SPEC D5): the database
// may become ready after the pod, and crashing instead would only add restart noise.
package retry

import (
	"context"
	"fmt"
	"time"
)

// Backoff doubles the wait after each failure, starting at Initial and capped at Max.
type Backoff struct {
	Initial time.Duration
	Max     time.Duration
}

// Until calls attempt until it returns nil or ctx is done. Before each wait it reports the
// failure and the wait to onFailure (e.g. to log it).
//
//	err := retry.Until(ctx, retry.Backoff{Initial: time.Second, Max: 10 * time.Second}, connect, logFailure)
func Until(ctx context.Context, backoff Backoff, attempt func(context.Context) error, onFailure func(err error, wait time.Duration)) error {
	wait := backoff.Initial
	for {
		err := attempt(ctx)
		if err == nil {
			return nil
		}
		onFailure(err, wait)
		if sleepErr := sleep(ctx, wait); sleepErr != nil {
			return fmt.Errorf("retry stopped: %w; last attempt error: %w", sleepErr, err)
		}
		wait = min(wait*2, backoff.Max)
	}
}

// sleep waits for d, returning early with ctx's error if ctx ends first.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
