package server

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/GEHACK/balloons/internal/printer"
)

// Retry policy for a ticket the printer refused. Every realistic failure here
// is transient and self-healing once a human notices — the printer is off, out
// of paper, unplugged from the switch, or already busy with another ticket —
// so a single failed attempt is no reason to drop a balloon on the floor.
const (
	printRetryBase = 2 * time.Second
	printRetryMax  = 60 * time.Second
	// printMaxAttempts bounds the total retry window at roughly 25 minutes
	// (2+4+8+16+32s, then a 60s floor). After that the ticket is dropped with
	// a loud log line and the UI's Reprint button is the manual escape hatch.
	printMaxAttempts = 30
	// printAttemptTimeout bounds one attempt: loom map fetch + typst render +
	// the write to the printer. Deliberately generous — ESC/POS refuses to
	// start a raster with less than 20s of budget left, and failing that guard
	// would turn a slow render into a permanent "skipped ticket".
	printAttemptTimeout = 60 * time.Second
)

// printJob is one ticket's place in the queue: the payload, how many attempts
// it has burned, and the earliest time it may be tried again.
type printJob struct {
	ticket   printer.Ticket
	force    bool // skip the already-printed check (Reprint)
	attempts int
	readyAt  time.Time
	// again is set when the ticket is re-requested while its attempt is
	// already in flight. The worker honours it after the attempt returns, so
	// a Reprint pressed mid-print isn't swallowed.
	again bool
}

// printQueue funnels every print through a single worker and retries failures
// with exponential backoff.
//
// Serializing is not just about retries: the ESC/POS module is a one-connection
// -at-a-time serial device, and the old "one goroutine per balloon" dispatch
// made every ticket race for the same lock with its own 30s deadline — so a
// burst of balloons could see the tickets at the back of the queue time out
// before they ever reached the wire. One worker means one attempt in flight,
// each with a full budget.
type printQueue struct {
	attempt func(ctx context.Context, t printer.Ticket, force bool) error

	// Retry policy, seeded from the constants above. Fields rather than
	// constants so tests can run the same logic on millisecond delays.
	retryBase      time.Duration
	retryMax       time.Duration
	maxAttempts    int
	attemptTimeout time.Duration

	mu      sync.Mutex
	jobs    []*printJob
	pending map[int64]*printJob // includes the in-flight job, for dedupe
	wake    chan struct{}
}

func newPrintQueue(attempt func(context.Context, printer.Ticket, bool) error) *printQueue {
	return &printQueue{
		attempt:        attempt,
		retryBase:      printRetryBase,
		retryMax:       printRetryMax,
		maxAttempts:    printMaxAttempts,
		attemptTimeout: printAttemptTimeout,
		pending:        map[int64]*printJob{},
		wake:           make(chan struct{}, 1),
	}
}

// add queues a ticket for printing. A balloon already queued (or in flight) is
// not duplicated: its payload is refreshed and its backoff reset, so a Reprint
// on a ticket that is still retrying jumps it back to the front of the line
// instead of piling up a second copy.
func (q *printQueue) add(t printer.Ticket, force bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if job, ok := q.pending[t.BalloonID]; ok {
		job.ticket = t
		job.force = job.force || force
		job.attempts = 0
		job.readyAt = time.Time{}
		job.again = true
		q.notify()
		return
	}
	job := &printJob{ticket: t, force: force}
	q.jobs = append(q.jobs, job)
	q.pending[t.BalloonID] = job
	q.notify()
}

// notify nudges the worker. Callers must hold q.mu.
func (q *printQueue) notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *printQueue) run(ctx context.Context) {
	for {
		job, wait := q.next()
		if job != nil {
			q.runJob(ctx, job)
			continue
		}
		// wait == 0 means the queue is empty: sleep until something is added.
		if wait <= 0 {
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
			}
			continue
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-q.wake:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// next pops the first job whose backoff has elapsed. When nothing is ready it
// returns the time until the soonest one is, or 0 if the queue is empty.
func (q *printQueue) next() (*printJob, time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	var soonest time.Duration
	for i, job := range q.jobs {
		if !job.readyAt.After(now) {
			q.jobs = append(q.jobs[:i], q.jobs[i+1:]...)
			job.again = false
			return job, 0
		}
		if d := job.readyAt.Sub(now); soonest == 0 || d < soonest {
			soonest = d
		}
	}
	return nil, soonest
}

// runJob makes one attempt and decides what happens to the job afterwards:
// done, re-requested, retried after a backoff, or given up on.
func (q *printQueue) runJob(ctx context.Context, job *printJob) {
	// Snapshot under the lock: add() may rewrite this job's ticket while the
	// attempt is running (a Reprint landing mid-print).
	q.mu.Lock()
	t, force := job.ticket, job.force
	id := t.BalloonID
	q.mu.Unlock()

	attemptCtx, cancel := context.WithTimeout(ctx, q.attemptTimeout)
	err := q.attempt(attemptCtx, t, force)
	cancel()

	q.mu.Lock()
	defer q.mu.Unlock()

	job.attempts++

	switch {
	case job.again:
		// Re-requested mid-print. Honour it regardless of how this attempt
		// went — that's what the runner asked for by hitting Reprint.
		job.again = false
		job.attempts = 0
		job.readyAt = time.Time{}
		q.jobs = append(q.jobs, job)
		q.notify()

	case err == nil:
		delete(q.pending, id)

	case ctx.Err() != nil:
		// Shutting down, not a printer problem. The ticket is still unprinted
		// in the state store, so the next start picks it up.
		log.Printf("print balloon %d: abandoned at shutdown: %v", id, err)
		delete(q.pending, id)

	case job.attempts >= q.maxAttempts:
		log.Printf("print balloon %d: giving up after %d attempts: %v (use Reprint once the printer is back)", id, job.attempts, err)
		delete(q.pending, id)

	default:
		delay := q.retryDelay(job.attempts)
		job.readyAt = time.Now().Add(delay)
		q.jobs = append(q.jobs, job)
		q.notify()
		log.Printf("print balloon %d: attempt %d/%d failed: %v (retrying in %s)", id, job.attempts, q.maxAttempts, err, delay)
	}
}

// retryDelay doubles from retryBase up to retryMax. attempt is 1-based (the
// delay after the first failure).
func (q *printQueue) retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 20 { // guard the shift; long past the cap anyway
		return q.retryMax
	}
	d := q.retryBase << (attempt - 1)
	if d > q.retryMax {
		return q.retryMax
	}
	return d
}
