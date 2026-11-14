package resilience

import "context"

type RoundRobin struct {
	Strategy Client
}

func (c RoundRobin) Do(ctx context.Context, senders []Sender, b Budget) (Response, error) {
	return rotate(ctx, senders, b, c.Strategy, nil)
}

func rotate(ctx context.Context, senders []Sender, b Budget, strategy Client, breaker *CircuitBreaker) (Response, error) {
	if len(senders) == 0 {
		return Response{}, ErrConfig
	}
	ctx, state, cancel, err := begin(ctx, b)
	if err != nil {
		return Response{}, err
	}
	defer cancel()
	if strategy == nil {
		strategy = Default{}
	}
	pool := rotation{nodes: make([]node, len(senders)), breaker: breaker}
	state.next = func(ctx context.Context) (target, error) {
		i, err := pool.choose(ctx)
		if err != nil {
			return target{}, err
		}
		return target{sender: senders[i], done: func(failed bool) { pool.record(i, failed) }}, nil
	}
	return retry(ctx, state, func() (Response, error) {
		return strategy.Do(ctx, senders[0], b)
	}, nil)
}
