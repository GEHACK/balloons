package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/GEHACK/balloons/gen/balloons/v1/balloonsv1connect"
	"github.com/GEHACK/balloons/internal/domjudge"
	"github.com/GEHACK/balloons/internal/loom"
	"github.com/GEHACK/balloons/internal/printer"
	"github.com/GEHACK/balloons/internal/server"
	"github.com/GEHACK/balloons/internal/state"
)

// config holds every value read from the process environment. Built once at
// startup by loadConfig; consumed by main().
type config struct {
	addr        string
	dj          *domjudge.Client
	stateDB     string
	hideGroups  []string
	noFSGroups  []string
	scanBaseURL string
	loomBaseURL string
	loc         *time.Location
}

func loadConfig() (config, error) {
	addr := getenv("ADDR", ":8080")

	loc, err := resolveTZ(os.Getenv("CONTEST_TZ"))
	if err != nil {
		return config{}, err
	}

	return config{
		addr: addr,
		dj: domjudge.New(
			mustEnv("DOMJUDGE_URL"),
			mustEnv("DOMJUDGE_USER"),
			mustEnv("DOMJUDGE_PASS"),
			mustEnv("DOMJUDGE_CONTEST_ID"),
		),
		stateDB:     getenv("STATE_DB", "balloons.db"),
		hideGroups:  parseCSV(os.Getenv("HIDE_GROUP_IDS")),
		noFSGroups:  parseCSV(os.Getenv("NO_FIRST_SOLVE_GROUP_IDS")),
		scanBaseURL: resolveScanBaseURL(os.Getenv("SCAN_BASE_URL"), addr),
		loomBaseURL: getenv("LOOM_BASE_URL", "https://loom.gehack.nl"),
		loc:         loc,
	}, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("scan base URL: %s", cfg.scanBaseURL)
	log.Printf("ticket timezone: %s", cfg.loc)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	p, err := printer.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	store, err := state.Open(cfg.stateDB)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	var lm *loom.Client
	if cfg.loomBaseURL != "" {
		lm = loom.New(cfg.loomBaseURL)
		log.Printf("loom map source: %s", cfg.loomBaseURL)
	}

	hub := server.NewHub(cfg.dj, p, store, lm, cfg.hideGroups, cfg.noFSGroups, cfg.scanBaseURL, cfg.loc)
	go hub.Run(ctx)

	svc := &server.Server{Hub: hub, DJ: cfg.dj, Store: store}
	path, handler := balloonsv1connect.NewBalloonServiceHandler(svc)

	mux := http.NewServeMux()
	mux.Handle(path, handler)
	mux.HandleFunc("/scan", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/scan.html")
	})
	mux.Handle("/", http.FileServer(http.Dir("web")))

	srv := &http.Server{Addr: cfg.addr, Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on %s, mounted %s", cfg.addr, path)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// resolveTZ parses an IANA timezone name (e.g. "Europe/Amsterdam") into a
// *time.Location for ticket datetime formatting. Empty string returns
// time.Local — fine on hosts that have their system TZ set correctly, but in
// containers/systemd units where the process inherits UTC, set CONTEST_TZ
// explicitly so ticket timestamps don't drift.
func resolveTZ(name string) (*time.Location, error) {
	if name == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("CONTEST_TZ %q: %w", name, err)
	}
	return loc, nil
}

// resolveScanBaseURL returns the configured SCAN_BASE_URL when set, otherwise
// derives one from os.Hostname() and addr so the QR on every ticket still
// points somewhere reachable on the contest LAN. The derived URL is only
// useful when the printing host is reachable from the runner's phone by
// hostname — set SCAN_BASE_URL explicitly when that's not the case (e.g.
// behind a reverse proxy, or when the host has a public DNS name).
func resolveScanBaseURL(env, addr string) string {
	if env != "" {
		return strings.TrimRight(env, "/")
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "localhost"
	}
	portSuffix := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		portSuffix = addr[i:]
	} else {
		portSuffix = ":" + addr
	}
	return "http://" + host + portSuffix
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env var: %s", k)
	}
	return v
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// parseCSV splits a comma-separated env-var value into a slice, trimming
// whitespace around each entry and dropping empties. Returns nil for an empty
// input so callers can range over it without a length check.
func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
