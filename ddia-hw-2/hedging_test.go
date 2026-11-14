package resilience

import (
	"context"
	"testing"
	"time"
)

func TestHedgingFastFirstResponse(t *testing.T) {
	first, second := sequence(200), sequence(200)
	response, err := (Hedging{Delay: time.Second}).Do(context.Background(), []Sender{first, second}, limits())
	requireSuccess(t, response, err)
	if first.calls() != 1 || second.calls() != 0 {
		t.Fatalf("calls=%d,%d", first.calls(), second.calls())
	}
}

func TestHedgingParallelFanoutAndLoserCancellation(t *testing.T) {
	firstStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	thirdStarted := make(chan struct{})
	thirdCanceled := make(chan struct{})
	first := senderFunc(func(ctx context.Context) (Response, error) {
		close(firstStarted)
		<-ctx.Done()
		close(firstCanceled)
		return Response{}, ctx.Err()
	})
	second := senderFunc(func(ctx context.Context) (Response, error) {
		select {
		case <-thirdStarted:
			return Response{Status: 201}, nil
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	})
	third := senderFunc(func(ctx context.Context) (Response, error) {
		close(thirdStarted)
		<-ctx.Done()
		close(thirdCanceled)
		return Response{}, ctx.Err()
	})
	start := time.Now()
	delay := 20 * time.Millisecond
	response, err := (Hedging{Delay: delay}).Do(context.Background(), []Sender{first, second, third}, limits())
	requireSuccess(t, response, err)
	if response.Status != 201 || time.Since(start) < delay {
		t.Fatal("hedging did not wait for its delay")
	}
	await(t, firstStarted)
	await(t, firstCanceled)
	await(t, thirdCanceled)
}

func TestHedgingIgnoresErrorAndWaitsForSuccess(t *testing.T) {
	b := Budget{Latency: 100 * time.Millisecond}
	first, second := sequence(400), sequence(200)
	delay := 10 * time.Millisecond
	start := time.Now()
	response, err := (Hedging{Delay: delay}).Do(context.Background(), []Sender{first, second}, b)
	requireSuccess(t, response, err)
	if first.calls() != 1 || second.calls() != 1 || time.Since(start) < delay {
		t.Fatalf("calls=%d,%d", first.calls(), second.calls())
	}
}

func TestHedgingAllFailuresWaitForLatencyBudget(t *testing.T) {
	b := Budget{Latency: 20 * time.Millisecond}
	first, second := sequence(400), sequence(503)
	start := time.Now()
	_, err := (Hedging{Delay: time.Millisecond}).Do(context.Background(), []Sender{first, second}, b)
	requireError(t, err, ErrLatency)
	if time.Since(start) < b.Latency || first.calls() != 1 || second.calls() != 1 {
		t.Fatal("hedging stopped early or retried a node")
	}
}

func TestHedgingCancellationAndValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	first := senderFunc(func(ctx context.Context) (Response, error) {
		cancel()
		<-ctx.Done()
		return Response{}, ctx.Err()
	})
	_, err := (Hedging{Delay: time.Second}).Do(ctx, []Sender{first}, limits())
	requireError(t, err, context.Canceled)
	_, err = (Hedging{}).Do(context.Background(), nil, limits())
	requireError(t, err, ErrConfig)
	_, err = (Hedging{Delay: -1}).Do(context.Background(), []Sender{sequence(200)}, limits())
	requireError(t, err, ErrConfig)
}
