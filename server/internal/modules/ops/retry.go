package ops

import "time"

// retryPolicy controls the delay between attempts of one fire.
type retryPolicy struct {
	base time.Duration
	max  time.Duration
}

// delay returns the wait after the failed attempt number failed (1-based):
// base, 2·base, 4·base, … capped at max.
func (p retryPolicy) delay(failed int) time.Duration {
	if failed < 1 || p.base <= 0 {
		return 0
	}
	d := p.base
	for i := 1; i < failed; i++ {
		if d >= p.max/2 {
			return p.max
		}
		d *= 2
	}
	return min(d, p.max)
}

// worstCase is the longest one fire can take: every attempt times out and
// every backoff is waited.
func (p retryPolicy) worstCase(timeout time.Duration, retries int) time.Duration {
	total := timeout * time.Duration(retries+1)
	for i := 1; i <= retries; i++ {
		total += p.delay(i)
	}
	return total
}
