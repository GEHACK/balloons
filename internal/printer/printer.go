// Package printer renders and delivers balloon tickets. The Printer interface
// abstracts over the delivery mechanism (IPP today, receipt printer later) so
// the hub can stay agnostic.
package printer

import (
	"context"
	"time"
)

type Ticket struct {
	BalloonID    int64
	ProblemLabel string
	TeamName     string
	TeamID       string
	FirstSolve   bool

	// AllProblems is the full set of problem labels in the contest, in
	// contest order. Used to render the strip of balloons on the ticket.
	AllProblems []string
	// Delivered are problem labels this team has already had a balloon
	// marked done for (excluding the current ticket).
	Delivered []string
	// InDelivery are problem labels this team has outstanding (not done).
	// Includes the current ticket's problem.
	InDelivery []string

	// IssuedAt is the timestamp printed on the ticket.
	IssuedAt time.Time

	// ScanURL is the runner-facing URL encoded into the ticket's QR code
	// (typically `<SCAN_BASE_URL>/scan?id=<BalloonID>`). Empty if the server
	// has no SCAN_BASE_URL configured; templates render a placeholder in
	// that case.
	ScanURL string

	// MapPath is a filesystem path to the per-team map image fetched from
	// loom. Empty if the loom fetch failed or wasn't configured; the
	// template omits the map block in that case.
	MapPath string
}

type Printer interface {
	Print(ctx context.Context, t Ticket) error
}
