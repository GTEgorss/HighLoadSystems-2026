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
	return retry(ctx, state, func() (Response, error) {
		i, err := pool.choose(ctx)
		if err != nil {
			return Response{}, err
		}
		response, err := strategy.Do(ctx, senders[i], b)
		pool.record(i, err != nil)
		return response, err
	}, nil)
}
