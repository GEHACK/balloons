package server

import (
	"errors"
	"strconv"

	balloonsv1 "github.com/GEHACK/balloons/gen/balloons/v1"
	"github.com/GEHACK/balloons/internal/state"
)

// ErrPrintJobNotFound is returned by MarkPrintJobDone for an id the hub has
// never seen.
var ErrPrintJobNotFound = errors.New("print job not found")

// PrintJobCode is what the cuproxy cover sheet encodes as a barcode and what
// the scan box accepts: "P" + id. The prefix keeps print jobs apart from
// balloon tickets, whose barcodes are bare balloon ids.
func PrintJobCode(id int64) string {
	return "P" + strconv.FormatInt(id, 10)
}

func printJobProto(j state.PrintJob) *balloonsv1.PrintJob {
	return &balloonsv1.PrintJob{
		Id:           j.ID,
		TeamName:     j.TeamName,
		RequestingIp: j.RequestingIP,
		Done:         j.Delivered,
	}
}

// LoadPrintJobs seeds the hub with every print job in the store. Call once
// before serving, so the first subscriber already sees jobs from before a
// restart.
func (h *Hub) LoadPrintJobs() error {
	jobs, err := h.store.ListPrintJobs()
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, j := range jobs {
		h.printJobs[j.ID] = printJobProto(j)
	}
	return nil
}

func (h *Hub) PrintJobSnapshot() []*balloonsv1.PrintJob {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.printJobSnapshotLocked()
}

func (h *Hub) printJobSnapshotLocked() []*balloonsv1.PrintJob {
	out := make([]*balloonsv1.PrintJob, 0, len(h.printJobs))
	for _, j := range h.printJobs {
		out = append(out, j)
	}
	return out
}

// AddPrintJob records a new print job and broadcasts it. Returns the job so
// the caller can hand its code back to cuproxy.
func (h *Hub) AddPrintJob(teamName, requestingIP string) (*balloonsv1.PrintJob, error) {
	rec, err := h.store.CreatePrintJob(teamName, requestingIP)
	if err != nil {
		return nil, err
	}
	j := printJobProto(rec)

	h.mu.Lock()
	defer h.mu.Unlock()
	h.printJobs[j.Id] = j
	h.broadcast([]*balloonsv1.StreamBalloonsResponse{{
		Kind:     balloonsv1.StreamBalloonsResponse_KIND_ADDED,
		PrintJob: j,
	}})
	return j, nil
}

// MarkPrintJobDone records delivery and broadcasts the update. Unlike a
// balloon there's no DOMjudge round-trip: the store is the source of truth.
// The lock is held across the (fast, local) store write so two runners
// scanning the same cover can't both broadcast.
func (h *Hub) MarkPrintJobDone(id int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	prev, ok := h.printJobs[id]
	if !ok {
		return ErrPrintJobNotFound
	}
	if prev.Done {
		return nil
	}
	if err := h.store.RecordPrintJobDelivered(id); err != nil {
		return err
	}
	// Replace rather than mutate: prev may still be queued in a subscriber
	// channel, waiting to be marshaled.
	j := &balloonsv1.PrintJob{
		Id:           prev.Id,
		TeamName:     prev.TeamName,
		RequestingIp: prev.RequestingIp,
		Done:         true,
	}
	h.printJobs[id] = j
	h.broadcast([]*balloonsv1.StreamBalloonsResponse{{
		Kind:     balloonsv1.StreamBalloonsResponse_KIND_UPDATED,
		PrintJob: j,
	}})
	return nil
}
