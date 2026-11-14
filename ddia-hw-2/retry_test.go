package resilience

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestRetriesRecoverFromEachFailureType(t *testing.T) {
	b := limits()
	b.SubrequestLatency = 20 * time.Millisecond
	s := sequence(400, 503, 0, 200)
	response, err := (Retries{}).Do(context.Background(), s, b)
	requireSuccess(t, response, err)
	if s.calls() != 4 {
		t.Fatalf("calls=%d", s.calls())
	}
}

func TestRetriesRespectEachBudget(t *testing.T) {
	for _, name := range []string{"requests", "fast", "failures", "timeouts"} {
		t.Run(name, func(t *testing.T) {
			b := limits()
			status := 503
			switch name {
			case "requests":
				b.MaxRequests = 2
			case "fast":
				b.FastErrors, status = 1, 400
			case "failures":
				b.Failures = 1
			case "timeouts":
				b.Timeouts, status = 1, 0
				b.SubrequestLatency = 10 * time.Millisecond
			}
			s := sequence(status, status, 200)
			_, err := (Retries{}).Do(context.Background(), s, b)
			requireError(t, err, ErrBudget)
			if s.calls() != 2 {
				t.Fatalf("calls=%d", s.calls())
			}
		})
	}
}

func TestRetriesZeroBudgetStopsAfterFirstFailure(t *testing.T) {
	b := limits()
	b.Failures = 0
	s := sequence(503, 200)
	_, err := (Retries{}).Do(context.Background(), s, b)
	requireError(t, err, ErrBudget)
	if s.calls() != 1 {
		t.Fatalf("calls=%d", s.calls())
	}
}

func TestRetriesTotalDeadline(t *testing.T) {
	b := limits()
	b.Latency = 45 * time.Millisecond
	b.SubrequestLatency = 20 * time.Millisecond
	s := sequence(0)
	_, err := (Retries{}).Do(context.Background(), s, b)
	requireError(t, err, ErrLatency)
	if s.calls() < 2 || s.calls() > 3 {
		t.Fatalf("calls=%d", s.calls())
	}
}

func TestBackoffDoublesDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := sequence(503, 503, 503, 200)
		c := Backoff{InitialDelay: 15 * time.Millisecond}
		response, err := c.Do(context.Background(), s, limits())
		requireSuccess(t, response, err)
		for i, gap := range s.gaps() {
			want := c.InitialDelay * time.Duration(1<<i)
			if gap != want {
				t.Fatalf("gap[%d]=%v, want %v", i, gap, want)
			}
		}
	})
}

func TestBackoffDoesNotDelayFastErrors(t *testing.T) {
	s := sequence(400, 200)
	b := limits()
	b.Latency = 100 * time.Millisecond
	c := Backoff{InitialDelay: time.Second}
	response, err := c.Do(context.Background(), s, b)
	requireSuccess(t, response, err)
}

func TestBackoffDelaysTimeoutRetry(t *testing.T) {
	s := sequence(0, 200)
	b := limits()
	b.SubrequestLatency = 20 * time.Millisecond
	c := Backoff{InitialDelay: 20 * time.Millisecond}
	response, err := c.Do(context.Background(), s, b)
	requireSuccess(t, response, err)
	if s.gaps()[0] < 40*time.Millisecond {
		t.Fatalf("gap=%v", s.gaps()[0])
	}
}

func TestBackoffWaitingConsumesLatencyBudget(t *testing.T) {
	b := limits()
	b.Latency = 20 * time.Millisecond
	s := sequence(503, 200)
	_, err := (Backoff{InitialDelay: time.Second}).Do(context.Background(), s, b)
	requireError(t, err, ErrLatency)
	if s.calls() != 1 {
		t.Fatalf("calls=%d", s.calls())
	}
}

func TestBackoffDoesNotWaitAfterLastAttempt(t *testing.T) {
	b := limits()
	b.MaxRequests = 1
	s := sequence(503)
	_, err := (Backoff{InitialDelay: time.Second}).Do(context.Background(), s, b)
	requireError(t, err, ErrBudget)
	if errors.Is(err, ErrLatency) {
		t.Fatal("waited after exhausting request budget")
	}
	_, err = (Backoff{}).Do(context.Background(), s, b)
	requireError(t, err, ErrConfig)
}
