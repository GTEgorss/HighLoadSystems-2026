package resilience

import (
	"context"
	"math"
	"testing"
	"time"
)

func breaker() CircuitBreaker {
	return CircuitBreaker{Window: 1, ErrorThreshold: .5, Cooldown: 20 * time.Millisecond}
}

func TestCircuitBreakerWindowAndStrictThreshold(t *testing.T) {
	c := breaker()
	c.Window = 2
	pool := rotation{nodes: make([]node, 1), breaker: &c}
	pool.record(0, false)
	pool.record(0, true)
	if !pool.nodes[0].openUntil.IsZero() {
		t.Fatal("opened at, rather than above, the threshold")
	}
	pool.record(0, true)
	if pool.nodes[0].openUntil.IsZero() {
		t.Fatal("did not discard the older successful result")
	}
}

func TestCircuitBreakerRecoveryAfterCooldown(t *testing.T) {
	c := breaker()
	s := sequence(503, 200)
	response, err := c.Do(context.Background(), []Sender{s}, limits())
	requireSuccess(t, response, err)
	if s.calls() != 2 || s.gaps()[0] < c.Cooldown {
		t.Fatalf("calls=%d gaps=%v", s.calls(), s.gaps())
	}
}

func TestCircuitBreakerAllOpenConsumesLatencyNotRequests(t *testing.T) {
	c := breaker()
	c.Cooldown = time.Second
	b := limits()
	b.Latency = 20 * time.Millisecond
	first, second := sequence(503), sequence(503)
	_, err := c.Do(context.Background(), []Sender{first, second}, b)
	requireError(t, err, ErrLatency)
	if first.calls() != 1 || second.calls() != 1 {
		t.Fatal("open nodes received extra requests")
	}
}

func TestCircuitBreakerCountsAllFailureTypes(t *testing.T) {
	for _, status := range []int{400, 503, 0} {
		c := breaker()
		b := limits()
		b.SubrequestLatency = 10 * time.Millisecond
		s := sequence(status, 200)
		response, err := c.Do(context.Background(), []Sender{s}, b)
		requireSuccess(t, response, err)
		if s.calls() != 2 || s.gaps()[0] < c.Cooldown {
			t.Fatalf("status=%d calls=%d gaps=%v", status, s.calls(), s.gaps())
		}
	}
}

func TestCircuitBreakerInvalidConfigurationAndCancellation(t *testing.T) {
	for _, c := range []CircuitBreaker{{}, {Window: 1, ErrorThreshold: math.NaN(), Cooldown: time.Second}, {Window: 1, ErrorThreshold: 1, Cooldown: time.Second}} {
		_, err := c.Do(context.Background(), []Sender{sequence(200)}, limits())
		requireError(t, err, ErrConfig)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := sequence(200)
	_, err := breaker().Do(ctx, []Sender{s}, limits())
	requireError(t, err, context.Canceled)
	if s.calls() != 0 {
		t.Fatal("sent a request after cancellation")
	}
}
