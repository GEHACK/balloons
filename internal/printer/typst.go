package printer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// typstInputs returns the `--input k=v` argument pairs shared by every Typst
// render in this package. Backend-specific args (page_width_mm, --format,
// --ppi) are passed via typstOpts.
func typstInputs(t Ticket) []string {
	issued := t.IssuedAt
	if issued.IsZero() {
		issued = time.Now()
	}
	return []string{
		"--input", "datetime=" + issued.Format("02-01-2006 15:04"),
		"--input", "ticket_id=" + strconv.FormatInt(t.BalloonID, 10),
		"--input", "problem=" + t.ProblemLabel,
		"--input", "team_name=" + t.TeamName,
		"--input", "team_id=" + t.TeamID,
		"--input", "balloons=" + strings.Join(t.AllProblems, ","),
		"--input", "delivered=" + strings.Join(t.Delivered, ","),
		"--input", "in_delivery=" + strings.Join(t.InDelivery, ","),
		"--input", "first_solve=" + strconv.FormatBool(t.FirstSolve),
		"--input", "scan_url=" + t.ScanURL,
		"--input", "map_path=" + t.MapPath,
	}
}

type typstOpts struct {
	template string
	ext      string   // output file extension: "pdf" or "png"
	format   string   // typst --format value; "" omits the flag (defaults to ext)
	ppi      float64  // typst --ppi; 0 omits the flag
	extra    []string // additional --input k=v pairs
}

// renderTypst compiles the template into a temp file under os.TempDir() and
// returns its path. The caller owns the file and must os.Remove it. On error
// the temp file (if any) is removed.
func renderTypst(ctx context.Context, t Ticket, opts typstOpts) (string, error) {
	out := filepath.Join(os.TempDir(), fmt.Sprintf("balloon-%d-%d.%s", t.BalloonID, time.Now().UnixNano(), opts.ext))
	// --root / so absolute paths (e.g. the per-ticket map fetched from loom
	// and written to os.TempDir()) resolve. Typst's default root is the
	// template's parent directory, which would reject anything under /tmp.
	args := []string{"compile", "--root", "/"}
	if opts.format != "" {
		args = append(args, "--format", opts.format)
	}
	if opts.ppi > 0 {
		args = append(args, "--ppi", strconv.FormatFloat(opts.ppi, 'f', 3, 64))
	}
	args = append(args, typstInputs(t)...)
	args = append(args, opts.extra...)
	args = append(args, opts.template, out)

	cmd := exec.CommandContext(ctx, "typst", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(out)
		return "", fmt.Errorf("printer: typst compile: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
