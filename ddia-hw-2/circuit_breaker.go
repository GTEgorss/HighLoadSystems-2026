package resilience

import (
	"context"
	"time"
)

type CircuitBreaker struct {
	Strategy       Client
	Window         int
	ErrorThreshold float64
	Cooldown       time.Duration
}

func (c CircuitBreaker) Do(ctx context.Context, senders []Sender, b Budget) (Response, error) {
	if c.Window < 1 || !(c.ErrorThreshold >= 0 && c.ErrorThreshold < 1) || c.Cooldown <= 0 {
		return Response{}, ErrConfig
	}
	return rotate(ctx, senders, b, c.Strategy, &c)
}

type node struct {
	history   []bool
	openUntil time.Time
}

type rotation struct {
	next    int
	nodes   []node
	breaker *CircuitBreaker
}

func (p *rotation) choose(ctx context.Context) (int, error) {
	for {
		if ctx.Err() != nil {
			return 0, contextError(ctx)
		}
		now := time.Now()
		var earliest time.Time
		for range p.nodes {
			i := p.next
			p.next = (p.next + 1) % len(p.nodes)
			if !now.Before(p.nodes[i].openUntil) {
				return i, nil
			}
			if earliest.IsZero() || p.nodes[i].openUntil.Before(earliest) {
				earliest = p.nodes[i].openUntil
			}
		}
		if err := wait(ctx, time.Until(earliest)); err != nil {
			return 0, err
		}
	}
}

func (p *rotation) record(i int, failed bool) {
	if p.breaker == nil {
		return
	}
	n := &p.nodes[i]
	n.history = append(n.history, failed)
	if len(n.history) > p.breaker.Window {
		n.history = n.history[1:]
	}
	if len(n.history) < p.breaker.Window {
		return
	}
	failures := 0
	for _, failure := range n.history {
		if failure {
			failures++
		}
	}
	if float64(failures)/float64(len(n.history)) > p.breaker.ErrorThreshold {
		n.openUntil = time.Now().Add(p.breaker.Cooldown)
		n.history = nil
	}
}
