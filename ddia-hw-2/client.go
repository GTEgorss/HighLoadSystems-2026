package resilience

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Response struct {
	Status int
	Body   []byte
}

type Sender interface {
	Send(context.Context) (Response, error)
}

type Client interface {
	Do(context.Context, Sender, Budget) (Response, error)
}

type MultiClient interface {
	Do(context.Context, []Sender, Budget) (Response, error)
}

type Budget struct {
	MaxRequests       int
	FastErrors        int
	Failures          int
	Timeouts          int
	Latency           time.Duration
	SubrequestLatency time.Duration
}

var (
	ErrBudget  = errors.New("request budget exhausted")
	ErrLatency = fmt.Errorf("%w: latency", ErrBudget)
	ErrTimeout = errors.New("subrequest timeout")
	ErrConfig  = errors.New("invalid configuration")
)

type HTTPError struct {
	Status int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d", e.Status)
}

type requestState struct {
	budget   Budget
	requests int
	fast     int
	failures int
	timeouts int
}

type stateKey struct{}

func begin(ctx context.Context, b Budget) (context.Context, *requestState, context.CancelFunc, error) {
	if state, ok := ctx.Value(stateKey{}).(*requestState); ok {
		return ctx, state, func() {}, nil
	}
	if b.MaxRequests < 1 || b.FastErrors < 0 || b.Failures < 0 || b.Timeouts < 0 || b.Latency <= 0 || b.SubrequestLatency <= 0 {
		return nil, nil, nil, ErrConfig
	}
	ctx, cancel := context.WithTimeout(ctx, b.Latency)
	state := &requestState{budget: b}
	return context.WithValue(ctx, stateKey{}, state), state, cancel, nil
}

func contextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.Join(ErrLatency, ctx.Err())
	}
	return ctx.Err()
}

func (s *requestState) acquire() error {
	if s.requests >= s.budget.MaxRequests {
		return ErrBudget
	}
	s.requests++
	return nil
}

func (s *requestState) retry(err error) error {
	var httpError *HTTPError
	switch {
	case errors.Is(err, ErrTimeout):
		s.timeouts++
		if s.timeouts > s.budget.Timeouts {
			return errors.Join(ErrBudget, err)
		}
	case errors.As(err, &httpError) && httpError.Status < 500:
		s.fast++
		if s.fast > s.budget.FastErrors {
			return errors.Join(ErrBudget, err)
		}
	case errors.As(err, &httpError):
		s.failures++
		if s.failures > s.budget.Failures {
			return errors.Join(ErrBudget, err)
		}
	default:
		return err
	}
	if s.requests >= s.budget.MaxRequests {
		return errors.Join(ErrBudget, err)
	}
	return nil
}

type Default struct{}

func (Default) Do(ctx context.Context, sender Sender, b Budget) (Response, error) {
	ctx, state, cancel, err := begin(ctx, b)
	if err != nil {
		return Response{}, err
	}
	defer cancel()
	if ctx.Err() != nil {
		return Response{}, contextError(ctx)
	}
	if err := state.acquire(); err != nil {
		return Response{}, err
	}
	callCtx, stop := context.WithCancel(ctx)
	defer stop()
	type result struct {
		response Response
		err      error
	}
	done := make(chan result, 1)
	started := time.Now()
	go func() {
		response, err := sender.Send(callCtx)
		done <- result{response, err}
	}()
	timer := time.NewTimer(state.budget.SubrequestLatency)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Response{}, contextError(ctx)
	case <-timer.C:
		return Response{}, ErrTimeout
	case outcome := <-done:
		if ctx.Err() != nil {
			return Response{}, contextError(ctx)
		}
		if time.Since(started) >= state.budget.SubrequestLatency {
			return Response{}, ErrTimeout
		}
		if outcome.err != nil {
			return outcome.response, outcome.err
		}
		if outcome.response.Status >= 200 && outcome.response.Status < 300 {
			return outcome.response, nil
		}
		if outcome.response.Status >= 400 && outcome.response.Status < 600 {
			return outcome.response, &HTTPError{Status: outcome.response.Status}
		}
		return outcome.response, fmt.Errorf("unexpected HTTP status: %d", outcome.response.Status)
	}
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return contextError(ctx)
	case <-timer.C:
		return contextError(ctx)
	}
}

func retry(ctx context.Context, state *requestState, send func() (Response, error), delay func(error) time.Duration) (Response, error) {
	for {
		response, err := send()
		if ctx.Err() != nil {
			return Response{}, contextError(ctx)
		}
		if err == nil || errors.Is(err, ErrBudget) {
			return response, err
		}
		if stopped := state.retry(err); stopped != nil {
			return response, stopped
		}
		if delay != nil {
			if err := wait(ctx, delay(err)); err != nil {
				return Response{}, err
			}
		}
	}
}
