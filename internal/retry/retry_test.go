package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	errTemporary = errors.New("temporary")
	errPermanent = errors.New("permanent")
)

func isTemporary(err error) bool {
	return errors.Is(err, errTemporary)
}

var noDelays = []time.Duration{0, 0, 0}

func failing(n int, err error) (fn func() error, calls *int) {
	calls = new(int)
	return func() error {
		*calls++
		if *calls <= n {
			return err
		}
		return nil
	}, calls
}

func TestDoSucceedsFirstTime(t *testing.T) {
	fn, calls := failing(0, errTemporary)

	assert.NoError(t, Do(t.Context(), noDelays, isTemporary, fn))
	assert.Equal(t, 1, *calls)
}

func TestDoRetriesUntilSuccess(t *testing.T) {
	fn, calls := failing(2, errTemporary)

	assert.NoError(t, Do(t.Context(), noDelays, isTemporary, fn))
	assert.Equal(t, 3, *calls)
}

func TestDoGivesUpAfterThreeRetries(t *testing.T) {
	fn, calls := failing(10, errTemporary)

	assert.ErrorIs(t, Do(t.Context(), noDelays, isTemporary, fn), errTemporary)
	assert.Equal(t, 4, *calls)
}

func TestDoDoesNotRetryPermanentError(t *testing.T) {
	fn, calls := failing(10, errPermanent)

	assert.ErrorIs(t, Do(t.Context(), noDelays, isTemporary, fn), errPermanent)
	assert.Equal(t, 1, *calls)
}

func TestDoStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	fn, calls := failing(10, errTemporary)

	start := time.Now()
	err := Do(ctx, Delays(), isTemporary, fn)

	assert.Less(t, time.Since(start), Delays()[0])
	assert.ErrorIs(t, err, errTemporary)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, *calls)
}

func TestDelays(t *testing.T) {
	assert.Equal(t, []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}, Delays())
}

func TestTotal(t *testing.T) {
	assert.Equal(t, 9*time.Second, Total(Delays()))
	assert.Zero(t, Total(nil))
}
