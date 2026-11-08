package retry

import (
	"context"
	"errors"
	"time"
)

func Delays() []time.Duration {
	return []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}
}

func Total(delays []time.Duration) time.Duration {
	var total time.Duration
	for _, d := range delays {
		total += d
	}

	return total
}

func Do(ctx context.Context, delays []time.Duration, retriable func(error) bool, fn func() error) error {
	err := fn()

	for _, delay := range delays {
		if err == nil || !retriable(err) {
			return err
		}

		if werr := wait(ctx, delay); werr != nil {
			return errors.Join(err, werr)
		}

		err = fn()
	}

	return err
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
