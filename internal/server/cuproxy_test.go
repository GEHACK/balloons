package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	balloonsv1 "github.com/GEHACK/balloons/gen/balloons/v1"
	"github.com/GEHACK/balloons/internal/loom"
	"github.com/GEHACK/balloons/internal/state"
)

func newTestHub(t *testing.T, dbPath string) *Hub {
	return newTestHubWithLoom(t, dbPath, nil)
}

func newTestHubWithLoom(t *testing.T, dbPath string, lm *loom.Client) *Hub {
	t.Helper()
	store, err := state.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	h := NewHub(nil, nil, store, lm, nil, nil, "", nil)
	if err := h.LoadPrintJobs(); err != nil {
		t.Fatal(err)
	}
	return h
}

func postWebhook(t *testing.T, wh *CuproxyWebhook, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/cuproxy/print-job", strings.NewReader(body))
	req.Header.Set("X-Forwarded-For", "10.1.2.3")
	rec := httptest.NewRecorder()
	wh.ServeHTTP(rec, req)
	return rec
}

func TestCuproxyWebhookLifecycle(t *testing.T) {
	db := filepath.Join(t.TempDir(), "state.db")
	h := newTestHub(t, db)
	wh := &CuproxyWebhook{Hub: h}
	_, _, _, ch, cancel := h.Subscribe()
	defer cancel()

	// Shape of cuproxy's POST body: its whole banner data, all strings.
	rec := postWebhook(t, wh, `{"requesting_ip":"10.0.0.42","name":"Team Rocket","map":"/tmp/x.png"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: got %d: %s", rec.Code, rec.Body)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["print_job_id"] != "P1" {
		t.Fatalf("print_job_id = %q, want P1", resp["print_job_id"])
	}
	ev := <-ch
	if ev.Kind != balloonsv1.StreamBalloonsResponse_KIND_ADDED || ev.PrintJob.GetTeamName() != "Team Rocket" || ev.PrintJob.GetRequestingIp() != "10.0.0.42" {
		t.Fatalf("unexpected added event: %v", ev)
	}

	// cuproxy escapes only double quotes, so a backslash breaks its JSON.
	// The job must still be created, falling back to X-Forwarded-For.
	rec = postWebhook(t, wh, `{"name":"C:\quux"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"P2"`) {
		t.Fatalf("malformed body: got %d: %s", rec.Code, rec.Body)
	}
	if ev := <-ch; ev.PrintJob.GetRequestingIp() != "10.1.2.3" {
		t.Fatalf("fallback ip = %q, want 10.1.2.3", ev.PrintJob.GetRequestingIp())
	}

	if err := h.MarkPrintJobDone(1); err != nil {
		t.Fatal(err)
	}
	if ev := <-ch; ev.Kind != balloonsv1.StreamBalloonsResponse_KIND_UPDATED || !ev.PrintJob.GetDone() {
		t.Fatalf("unexpected updated event: %v", ev)
	}
	if err := h.MarkPrintJobDone(99); err != ErrPrintJobNotFound {
		t.Fatalf("unknown id: got %v, want ErrPrintJobNotFound", err)
	}

	// A restart reloads both jobs with their delivery state.
	jobs := newTestHub(t, db).PrintJobSnapshot()
	done := map[int64]bool{}
	for _, j := range jobs {
		done[j.Id] = j.Done
	}
	if len(jobs) != 2 || !done[1] || done[2] {
		t.Fatalf("reloaded jobs: %v", jobs)
	}
}

// With the hook in its own parallel group, cuproxy's body has no `name` on a
// machine's first print, so the webhook asks loom. An unseated machine (loom
// 404) still gets a job, just without a name.
func TestCuproxyWebhookLoomFallback(t *testing.T) {
	loomSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/team-info/10.0.0.42" {
			_, _ = w.Write([]byte(`{"id":"7","name":"Team Rocket","ip":"10.0.0.42"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer loomSrv.Close()

	h := newTestHubWithLoom(t, filepath.Join(t.TempDir(), "state.db"), loom.New(loomSrv.URL))
	wh := &CuproxyWebhook{Hub: h}
	_, _, _, ch, cancel := h.Subscribe()
	defer cancel()

	for _, tc := range []struct{ body, wantName string }{
		{`{"requesting_ip":"10.0.0.42"}`, "Team Rocket"},
		{`{"requesting_ip":"10.0.0.99"}`, ""},
		// A name cuproxy already has wins; loom isn't asked.
		{`{"requesting_ip":"10.0.0.42","name":"From Banner"}`, "From Banner"},
	} {
		if rec := postWebhook(t, wh, tc.body); rec.Code != http.StatusOK {
			t.Fatalf("%s: got %d: %s", tc.body, rec.Code, rec.Body)
		}
		if got := (<-ch).PrintJob.GetTeamName(); got != tc.wantName {
			t.Fatalf("%s: team name = %q, want %q", tc.body, got, tc.wantName)
		}
	}
}
