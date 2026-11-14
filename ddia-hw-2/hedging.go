package resilience

import (
	"context"
	"time"
)

type Hedging struct {
	Delay time.Duration
}

func (c Hedging) Do(ctx context.Context, senders []Sender, b Budget) (Response, error) {
	if len(senders) == 0 || c.Delay < 0 || b.Latency <= 0 {
		return Response{}, ErrConfig
	}
	ctx, cancel := context.WithTimeout(ctx, b.Latency)
	defer cancel()
	type result struct {
		response Response
		err      error
	}
	results := make(chan result, len(senders))
	launch := func(sender Sender) {
		go func() {
			one := Budget{MaxRequests: 1, Latency: b.Latency, SubrequestLatency: b.Latency}
			response, err := (Default{}).Do(ctx, sender, one)
			results <- result{response, err}
		}()
	}
	launch(senders[0])
	timer := time.NewTimer(c.Delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return Response{}, contextError(ctx)
		case outcome := <-results:
			if ctx.Err() != nil {
				return Response{}, contextError(ctx)
			}
			if outcome.err == nil {
				return outcome.response, nil
			}
		case <-timer.C:
			for _, sender := range senders[1:] {
				launch(sender)
			}
		}
	}
}
