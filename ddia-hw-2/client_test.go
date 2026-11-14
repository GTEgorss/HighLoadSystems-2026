package resilience

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type senderFunc func(context.Context) (Response, error)

func (f senderFunc) Send(ctx context.Context) (Response, error) {
	return f(ctx)
}

type script struct {
	mu       sync.Mutex
	statuses []int
	times    []time.Time
}

func sequence(statuses ...int) *script {
	return &script{statuses: statuses}
}

func (s *script) Send(ctx context.Context) (Response, error) {
	s.mu.Lock()
	i := min(len(s.times), len(s.statuses)-1)
	status := s.statuses[i]
	s.times = append(s.times, time.Now())
	s.mu.Unlock()
	if status == 0 {
		<-ctx.Done()
		return Response{}, ctx.Err()
	}
	return Response{Status: status, Body: []byte("payload")}, nil
}

func (s *script) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.times)
}

func (s *script) gaps() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	var gaps []time.Duration
	for i := 1; i < len(s.times); i++ {
		gaps = append(gaps, s.times[i].Sub(s.times[i-1]))
	}
	return gaps
}

func limits() Budget {
	return Budget{MaxRequests: 8, FastErrors: 8, Failures: 8, Timeouts: 8,
		Latency: time.Second, SubrequestLatency: 100 * time.Millisecond}
}

func requireSuccess(t *testing.T, response Response, err error) {
	t.Helper()
	if err != nil || response.Status < 200 || response.Status >= 300 {
		t.Fatalf("response=%+v error=%v", response, err)
	}
}

func requireError(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("error=%v, want %v", got, want)
	}
}

func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("sender was not canceled")
	}
}

func TestDefaultSuccess(t *testing.T) {
	s := sequence(201)
	response, err := (Default{}).Do(context.Background(), s, limits())
	requireSuccess(t, response, err)
	if string(response.Body) != "payload" || s.calls() != 1 {
		t.Fatalf("body=%q calls=%d", response.Body, s.calls())
	}
}

func TestDefaultDoesNotRetry(t *testing.T) {
	for _, status := range []int{400, 503} {
		s := sequence(status, 200)
		response, err := (Default{}).Do(context.Background(), s, limits())
		var httpError *HTTPError
		if !errors.As(err, &httpError) || httpError.Status != status || response.Status != status || s.calls() != 1 {
			t.Fatalf("response=%+v error=%v calls=%d", response, err, s.calls())
		}
	}
}

func TestDefaultTimeoutCancelsSender(t *testing.T) {
	canceled := make(chan struct{})
	s := senderFunc(func(ctx context.Context) (Response, error) {
		<-ctx.Done()
		close(canceled)
		return Response{}, ctx.Err()
	})
	b := limits()
	b.SubrequestLatency = 20 * time.Millisecond
	_, err := (Default{}).Do(context.Background(), s, b)
	requireError(t, err, ErrTimeout)
	await(t, canceled)
}

func TestDefaultTotalDeadlineAndCancellation(t *testing.T) {
	b := limits()
	b.Latency = 20 * time.Millisecond
	_, err := (Default{}).Do(context.Background(), sequence(0), b)
	requireError(t, err, ErrLatency)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := sequence(200)
	_, err = (Default{}).Do(ctx, s, limits())
	requireError(t, err, context.Canceled)
	if s.calls() != 0 {
		t.Fatal("sent a request after cancellation")
	}
}

func TestDefaultInvalidBudget(t *testing.T) {
	_, err := (Default{}).Do(context.Background(), sequence(200), Budget{})
	requireError(t, err, ErrConfig)
}
