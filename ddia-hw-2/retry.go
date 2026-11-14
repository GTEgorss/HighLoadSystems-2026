package resilience

import (
	"context"
	"errors"
	"time"
)

type Retries struct{}

func (Retries) Do(ctx context.Context, sender Sender, b Budget) (Response, error) {
	ctx, state, cancel, err := begin(ctx, b)
	if err != nil {
		return Response{}, err
	}
	defer cancel()
	return retry(ctx, state, func() (Response, error) {
		return (Default{}).Do(ctx, sender, b)
	}, nil)
}

type Backoff struct {
	InitialDelay time.Duration
}

func (c Backoff) Do(ctx context.Context, sender Sender, b Budget) (Response, error) {
	if c.InitialDelay <= 0 {
		return Response{}, ErrConfig
	}
	ctx, state, cancel, err := begin(ctx, b)
	if err != nil {
		return Response{}, err
	}
	defer cancel()
	nextDelay := c.InitialDelay
	return retry(ctx, state, func() (Response, error) {
		return (Default{}).Do(ctx, sender, b)
	}, func(err error) time.Duration {
		var httpError *HTTPError
		if errors.As(err, &httpError) && httpError.Status < 500 {
			return 0
		}
		delay := nextDelay
		if nextDelay > state.budget.Latency/2 {
			nextDelay = state.budget.Latency
		} else {
			nextDelay *= 2
		}
		return delay
	})
}
