package resilience

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestRoundRobinOrderAndWraparound(t *testing.T) {
	var mu sync.Mutex
	var order []int
	var senders []Sender
	for i, s := range []*script{sequence(503, 503), sequence(400, 200)} {
		senders = append(senders, senderFunc(func(ctx context.Context) (Response, error) {
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			return s.Send(ctx)
		}))
	}
	response, err := (RoundRobin{}).Do(context.Background(), senders, limits())
	requireSuccess(t, response, err)
	if !reflect.DeepEqual(order, []int{0, 1, 0, 1}) {
		t.Fatalf("order=%v", order)
	}
}

func TestRoundRobinStopsOnSuccess(t *testing.T) {
	first, second := sequence(200), sequence(200)
	response, err := (RoundRobin{}).Do(context.Background(), []Sender{first, second}, limits())
	requireSuccess(t, response, err)
	if first.calls() != 1 || second.calls() != 0 {
		t.Fatalf("calls=%d,%d", first.calls(), second.calls())
	}
}

func TestRoundRobinMovesAfterTimeout(t *testing.T) {
	first, second := sequence(0), sequence(200)
	b := limits()
	b.SubrequestLatency = 20 * time.Millisecond
	response, err := (RoundRobin{}).Do(context.Background(), []Sender{first, second}, b)
	requireSuccess(t, response, err)
	if first.calls() != 1 || second.calls() != 1 {
		t.Fatalf("calls=%d,%d", first.calls(), second.calls())
	}
}

func TestRoundRobinComposesBackoffWithSharedBudget(t *testing.T) {
	c := RoundRobin{Strategy: Backoff{InitialDelay: 10 * time.Millisecond}}
	first, second := sequence(503, 200), sequence(200)
	start := time.Now()
	response, err := c.Do(context.Background(), []Sender{first, second}, limits())
	requireSuccess(t, response, err)
	if first.calls() != 1 || second.calls() != 1 || time.Since(start) < 10*time.Millisecond {
		t.Fatal("backoff or rotation was not applied")
	}
	b := limits()
	b.MaxRequests = 2
	first, second = sequence(503), sequence(503)
	_, err = c.Do(context.Background(), []Sender{first, second}, b)
	requireError(t, err, ErrBudget)
	if first.calls()+second.calls() != 2 {
		t.Fatal("nested strategy reset the request budget")
	}
	b = limits()
	b.Latency = 20 * time.Millisecond
	c.Strategy = Backoff{InitialDelay: time.Second}
	first, second = sequence(503, 200), sequence(200)
	_, err = c.Do(context.Background(), []Sender{first, second}, b)
	requireError(t, err, ErrLatency)
	if first.calls() != 1 || second.calls() != 0 {
		t.Fatal("nested strategy reset the latency budget")
	}
}

func TestComposedBackoffReachesHealthyNode(t *testing.T) {
	for _, name := range []string{"round-robin", "circuit-breaker"} {
		for _, status := range []int{503, 0} {
			t.Run(fmt.Sprintf("%s/%d", name, status), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					b := limits()
					b.MaxRequests = 2
					b.SubrequestLatency = 10 * time.Millisecond
					first, second := sequence(status), sequence(200)
					backoff := Backoff{InitialDelay: time.Millisecond}
					var client MultiClient = RoundRobin{Strategy: backoff}
					if name == "circuit-breaker" {
						client = CircuitBreaker{Strategy: backoff, Window: 1, ErrorThreshold: .5, Cooldown: time.Second}
					}
					response, err := client.Do(context.Background(), []Sender{first, second}, b)
					requireSuccess(t, response, err)
					if first.calls() != 1 || second.calls() != 1 {
						t.Fatalf("calls=%d,%d", first.calls(), second.calls())
					}
				})
			})
		}
	}
}

func TestRoundRobinEmptyPoolAndExhaustion(t *testing.T) {
	_, err := (RoundRobin{}).Do(context.Background(), nil, limits())
	requireError(t, err, ErrConfig)
	b := limits()
	b.MaxRequests = 3
	first, second := sequence(503), sequence(503)
	_, err = (RoundRobin{}).Do(context.Background(), []Sender{first, second}, b)
	requireError(t, err, ErrBudget)
	if first.calls() != 2 || second.calls() != 1 {
		t.Fatalf("calls=%d,%d", first.calls(), second.calls())
	}
}
