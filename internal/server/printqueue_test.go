package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GEHACK/balloons/internal/printer"
)

// testQueue builds a queue on millisecond delays so the retry logic can be
// exercised without waiting out the production backoff.
func testQueue(t *testing.T, attempt func(context.Context, printer.Ticket, bool) error) *printQueue {
	t.Helper()
	q := newPrintQueue(attempt)
	q.retryBase = time.Millisecond
	q.retryMax = 5 * time.Millisecond
	q.maxAttempts = 5
	q.attemptTimeout = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go q.run(ctx)
	return q
}

func ticket(id int64) printer.Ticket { return printer.Ticket{BalloonID: id} }

// waitFor polls cond until it holds or the test times out.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A printer that fails a few times then recovers must still get the ticket out.
func TestPrintQueueRetriesUntilSuccess(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	done := make(chan struct{})

	q := testQueue(t, func(context.Context, printer.Ticket, bool) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls < 3 {
			return errors.New("printer offline")
		}
		close(done)
		return nil
	})
	q.add(ticket(1), false)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ticket never printed")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 3 {
		t.Fatalf("attempts = %d, want 3", calls)
	}
}

// A printer that never comes back must not retry forever.
func TestPrintQueueGivesUpAfterMaxAttempts(t *testing.T) {
	var mu sync.Mutex
	calls := 0

	q := testQueue(t, func(context.Context, printer.Ticket, bool) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return errors.New("printer offline")
	})
	q.add(ticket(1), false)

	waitFor(t, "queue to drain", func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		return len(q.pending) == 0 && len(q.jobs) == 0
	})
	mu.Lock()
	defer mu.Unlock()
	if calls != q.maxAttempts {
		t.Fatalf("attempts = %d, want %d", calls, q.maxAttempts)
	}
}

// Only one attempt may be in flight at a time — the ESC/POS module accepts a
// single connection and interleaved rasters wedge it.
func TestPrintQueueSerializesAttempts(t *testing.T) {
	var mu sync.Mutex
	inFlight, maxInFlight, finished := 0, 0, 0

	q := testQueue(t, func(context.Context, printer.Ticket, bool) error {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()

		time.Sleep(2 * time.Millisecond)

		mu.Lock()
		inFlight--
		finished++
		mu.Unlock()
		return nil
	})
	for id := int64(1); id <= 8; id++ {
		q.add(ticket(id), false)
	}

	waitFor(t, "all tickets to print", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return finished == 8
	})
	mu.Lock()
	defer mu.Unlock()
	if maxInFlight != 1 {
		t.Fatalf("max concurrent attempts = %d, want 1", maxInFlight)
	}
}

// Re-queuing a balloon that is still waiting on a retry must not print it
// twice once the printer recovers.
func TestPrintQueueDedupesWhileRetrying(t *testing.T) {
	var mu sync.Mutex
	var forces []bool
	fail := true

	q := testQueue(t, func(_ context.Context, _ printer.Ticket, force bool) error {
		mu.Lock()
		forces = append(forces, force)
		failing := fail
		mu.Unlock()
		if failing {
			return errors.New("printer offline")
		}
		return nil
	})

	q.add(ticket(1), false)
	waitFor(t, "first attempt", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(forces) >= 1
	})

	// Runner hits Reprint while the ticket is still backing off.
	q.add(ticket(1), true)
	mu.Lock()
	fail = false
	mu.Unlock()

	waitFor(t, "queue to drain", func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		return len(q.pending) == 0 && len(q.jobs) == 0
	})

	mu.Lock()
	defer mu.Unlock()
	if got := forces[len(forces)-1]; !got {
		t.Fatal("last attempt was not forced; Reprint would hit the already-printed check")
	}
	// One queue slot, so at most maxAttempts tries total — a duplicate job
	// would keep attempting after the queue reports itself drained.
	if len(forces) > q.maxAttempts {
		t.Fatalf("attempts = %d, want no more than %d (duplicate job queued?)", len(forces), q.maxAttempts)
	}
}
