package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// CuproxyWebhook registers team prints that go through cuproxy (the IPP proxy
// that puts a cover sheet on every print job). Configure it as its own
// parallel group in cuproxy's WEBHOOKS_TO_CALL, e.g.
//
//	info;GET;https://loom…/api/team-info/{{requesting_ip}}&&printjob;POST;http://127.0.0.1:8090/api/cuproxy/print-job
//
// Not chained after `info` with `|`: cuproxy aborts the rest of a sequential
// chain when a hook fails, and loom's team-info 404s for a machine with no
// team seated at it, so chaining would silently drop those prints.
//
// cuproxy calls every hook once per print job, before rendering the cover.
// A POST hook gets cuproxy's banner data as a flat JSON object of strings:
// always `requesting_ip`, and `name` only if loom's team-info already
// answered (for this job or an earlier one from the same machine). Without
// it we ask loom ourselves. We create a print job and answer with
// {"print_job_id": "P<id>"}. cuproxy merges that into the banner data, and
// its Typst cover template prints the code as a barcode for the runner to
// scan.
//
// The endpoint is unauthenticated: balloons is only exposed on the admin
// network, and cuproxy reaches it over loopback.
type CuproxyWebhook struct {
	Hub *Hub
}

// cuproxyTeamNameKey is the banner-data key carrying the team's display
// name: the `name` field of loom's team-info, which the cover template prints.
const cuproxyTeamNameKey = "name"

func (c *CuproxyWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	// cuproxy builds this JSON by hand and escapes only double quotes, so a
	// backslash in any value makes it invalid. A print is happening either
	// way, so a bad body still gets a job; it just shows up without a team
	// name.
	data := map[string]any{}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&data); err != nil {
		log.Printf("cuproxy webhook: decode body: %v", err)
	}
	ip := str(data["requesting_ip"])
	if ip == "" {
		// cuproxy also sets X-Forwarded-For to the printing machine's IP.
		ip = strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	}

	name := str(data[cuproxyTeamNameKey])
	if name == "" {
		name = c.lookupTeamName(r.Context(), ip)
	}

	j, err := c.Hub.AddPrintJob(name, ip)
	if err != nil {
		log.Printf("cuproxy webhook: %v", err)
		http.Error(w, "could not record print job", http.StatusInternalServerError)
		return
	}
	code := PrintJobCode(j.Id)
	log.Printf("cuproxy webhook: print job %s for %q (%s)", code, j.TeamName, j.RequestingIp)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"print_job_id": code})
}

// lookupTeamName asks loom which team sits at ip. Best-effort: on any failure
// the job is still recorded, and the UI falls back to showing the IP.
func (c *CuproxyWebhook) lookupTeamName(ctx context.Context, ip string) string {
	if c.Hub.loom == nil || ip == "" {
		return ""
	}
	// cuproxy holds the print until every hook answers, so keep this short.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	name, err := c.Hub.loom.TeamName(ctx, ip)
	if err != nil {
		log.Printf("cuproxy webhook: team name for %s: %v", ip, err)
	}
	return name
}

func str(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}
