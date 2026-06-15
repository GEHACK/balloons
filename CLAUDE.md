# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Purpose

Custom replacement for DOMjudge's built-in balloon tool, written for the GEHACK contest. The built-in tool doesn't surface a first-solve flag and can't drive a receipt printer; this tool exists to fix both. A future contest-area map will also consume data from this service. The printer subsystem is abstracted (`internal/printer`) so the same hub can drive an IPP printer today and a receipt printer later; the map integration is still out of scope.

## Dev environment

Everything (Go, buf, protoc plugins, Node, Tailwind) is pinned in `shell.nix`. Enter with `nix-shell`, or wrap one-off commands with `nix-shell --run "..."`. Commands below assume the shell is active.

Required env vars (see `.env.example`): `DOMJUDGE_URL`, `DOMJUDGE_USER`, `DOMJUDGE_PASS`, `DOMJUDGE_CONTEST_ID`. Load with `set -a; source .env; set +a` before running the server. Optional: `ADDR` (default `:8080`), `HIDE_GROUP_IDS` (CSV of DOMjudge group ids whose balloons disappear entirely), `NO_FIRST_SOLVE_GROUP_IDS` (CSV of group ids whose teams still get balloons but never the first-solve flag), `PRINTER_KIND` (`escpos` default, or `ipp`), `PRINTER_IPP_URI` (full `ipp://host:port/queue` URI; required when `PRINTER_KIND=ipp`), `PRINTER_ESCPOS_ADDR` (`host:port` of the thermal printer's raw socket, typically `:9100`; required when `PRINTER_KIND=escpos`), `PRINTER_ESCPOS_WIDTH` (printer head width in dots, default `576` for 80mm/203dpi), `PRINTER_TEMPLATE` (default `templates/balloon.typ` — a single thermal-receipt layout, used by both backends), `CONTEST_TZ` (IANA timezone name like `Europe/Amsterdam` used to render the ticket datetime; defaults to the server process's `time.Local` — set explicitly when running in containers/systemd to avoid UTC drift), `STATE_DB` (path to the SQLite ticket-state file, default `balloons.db`), `SCAN_BASE_URL` (public base URL used to build the per-ticket QR code; when unset, falls back to `http://<os.Hostname()><ADDR>` so the QR still prints on a contest LAN — set explicitly when behind a reverse proxy or when the runner's phone can't resolve the host by name), `LOOM_BASE_URL` (base URL of the loom service that serves per-team map images at `/api/map-image?team_id=<id>`; defaults to `https://loom.gehack.nl`. If the fetch fails for a given team, the ticket prints without the map block rather than failing the print).

## Common commands

A `justfile` wraps everything. `just` lists recipes; the useful ones are `just bootstrap` (first-time setup), `just gen` (regen Go + TS from proto), `just build-web`, `just watch`, `just run`, `just fmt`, and `just lint`. `set dotenv-load` is on so `.env` is read automatically for any recipe.

There are no automated tests; the service is exercised by pointing the server at a live DOMjudge contest. If a printed ticket gets lost in transit, use the **Reprint** button in the UI — it clears the local `printed_at` row for that balloon and re-dispatches the print goroutine.

## Architecture

**Single Go binary** at `cmd/server` serves both the connectRPC API and the static frontend on one port. `mux.Handle(path, handler)` mounts connectRPC; `mux.Handle("/", http.FileServer(http.Dir("web")))` serves `web/index.html` and `web/dist/*` for everything else.

**Hub pattern** (`internal/server/hub.go`) is the cache + fan-out:
- `Run(ctx)` does an initial `refresh()` then spawns `runEventFeed()` and serializes subsequent refreshes off a buffered `trigger` channel.
- `refresh()` fetches `/balloons`, `/teams`, `/state`, and `/problems` from DOMjudge, applies group-based filters (hide + no-first-solve), rebuilds the proto state, diffs against the previous map, and broadcasts `KIND_ADDED`/`KIND_UPDATED` events to all subscribers. It also precomputes the contest's full label strip plus per-team delivered / in-delivery label sets up front so dispatched print goroutines don't need the hub lock.
- `runEventFeed()` holds a long-lived NDJSON read on `/api/v4/contests/{cid}/event-feed?stream=true&types=judgements,balloons,state` and calls `TriggerRefresh()` on every line. Reconnect with exponential backoff up to 30s.
- Subscribers get a snapshot + a buffered channel from `Subscribe()`. Slow subscribers get force-closed (they reconnect and pick up a fresh snapshot).

`MarkDone` calls DOMjudge then `TriggerRefresh()` so the UI sees the change within one round-trip instead of waiting for the event-feed. It also calls `Store.RecordDelivered` so the local SQLite reflects delivery completion.

**Printer abstraction** (`internal/printer`) is an interface (`Printer { Print(ctx, Ticket) error }`) with two implementations: `IPP` (renders `templates/balloon.typ` via the `typst` CLI into a PDF, then submits it to an IPP queue via `phin1x/go-ipp`); and `ESCPOS` (renders the template at 2× supersampling, box-filters down to the configured dot width, converts each pixel to 1-bit using a chroma-aware ink-density pass — saturated colors snap to solid black, near-grayscale goes through Floyd-Steinberg — then streams the result as `GS v 0` raster chunks plus a partial-cut over a TCP raw socket, typically port 9100). The Typst page width is derived as `width / 203dpi` and passed to the template via `--input page_width_mm=...`; the template reads that input so adjusting `PRINTER_ESCPOS_WIDTH` does not require template edits. The shared `typstInputs` helper (`internal/printer/typst.go`) builds the common `--input k=v` pairs both backends pass to `typst compile`. If you swap to a printer with a different head DPI, change `targetDPI` in `escpos.go`. The hub fires `go h.print(ticket)` on every `KIND_ADDED` event for a non-done balloon. Reprints are gated by the **state store** (`internal/state`, a small SQLite table): `h.print` calls `Store.IsPrinted(id)` before printing and `Store.RecordPrinted(id)` after, so restarting the server never reprints already-printed balloons and concurrent refreshes can't double-print. Deleting `STATE_DB` resets the dedupe.

**Frontend** is a single HTML page + bundled TS (`web/src/main.ts`). It opens a server-streaming `StreamBalloons` RPC and renders straight from event deltas (no `ListBalloons` call in the happy path). On stream error it reconnects with the same exponential backoff. State is a `Map<string, Balloon>` keyed by id-as-string.

**Runner scan flow** lives in `web/scan.html`, served at `GET /scan?id=<balloon_id>`. It's intentionally a standalone page (no shared bundle, no connect-web SDK) so a phone with weak signal can load it fast — it calls `ListBalloons` and `MarkDone` directly as plain Connect-protocol JSON POSTs (`fetch("/balloons.v1.BalloonService/MarkDone", ...)`). The QR on every printed ticket encodes `<SCAN_BASE_URL>/scan?id=<n>`; on load the page shows "Delivered ✓" immediately and a 5-second Undo button, then fires `MarkDone` once the timer expires. The 5-second buffer matters because DOMjudge's `done` is one-way (see gotchas) — once we POST the mark, we can't take it back, so the only honest "undo" is "cancel before we commit." A balloon that's already `done` at scan time shows "Already delivered" and no countdown.

## DOMjudge integration gotchas

These cost time to figure out — keep them in mind:

- **First-solve must be derived from `/balloons` itself.** DOMjudge's `/awards` endpoint is empty during a live contest (it's only populated post-contest), so we can't use it. Per problem, the balloon with the earliest `time` is the first solve (`firstSolveIDs` in `internal/server/server.go`). The `time` field is a fixed-width seconds.nanoseconds string, so lexical compare is correct. Teams whose group is in `NO_FIRST_SOLVE_GROUP_IDS` are skipped — if a company team solves first, the next eligible team gets the flag.
- **Team groups come from `/teams`, not `/balloons`.** The balloon JSON has `categoryid: null` even when the team has a category. Fetch `/api/v4/contests/{cid}/teams`, each team has a `group_ids` array. Filters (hide + no-first-solve) match if *any* of a team's `group_ids` is in the filter set.
- **`done` is one-way.** DOMjudge only exposes `POST /balloons/{id}/done` — there is no unmark endpoint in the API. Current behavior matches that limitation; if undo becomes a requirement, track delivered state locally rather than reverse-engineering the admin web route.
- **`team` is `{label}: {name}`.** DOMjudge prepends the team's label (number or string) to the display name. Stripped server-side in `toProto` with `^\S+:\s+`.
- **Event-feed events are triggers, not deltas.** The code treats any event as "something changed, refetch and diff." Don't try to interpret event payloads — `/balloons`, `/teams`, and `/state` are canonical.
- **Freeze detection comes from `/state`.** Scoreboard freeze is active when `frozen != null && thawed == null` (`State.FrozenNow` in `internal/domjudge/client.go`). The hub broadcasts a `KIND_FREEZE` event on transitions and on every new subscription so reloads pick up the current state.

## Proto / wire surface

`proto/balloons/v1/balloons.proto` is deliberately minimal: 6 fields on `Balloon` (`id`, `problem_label`, `problem_rgb`, `team_name`, `done`, `first_solve`). `StreamBalloonsResponse` carries a `Kind` (`ADDED`/`UPDATED`/`FREEZE`) plus an optional `balloon` and a `frozen` bool used only on `KIND_FREEZE` events. The server holds more DOMjudge data internally but doesn't put it on the wire. Add fields when a consumer needs them, not preemptively.

Generated code (`gen/` for Go, `web/src/gen/` for TS) is gitignored. Always run `buf generate` after editing the proto.

## What's where

```
proto/balloons/v1/         schema (single file)
gen/                       generated Go (gitignored)
web/src/gen/               generated TS (gitignored)
cmd/server/                main.go — env config + http.Server + hub.Run()
internal/domjudge/         REST + event-feed client; mirrors DOMjudge JSON shapes
internal/server/           connectRPC handlers + Hub
internal/printer/          Printer interface + IPP and ESCPOS impls
internal/state/            SQLite-backed ticket-state store (printed_at, delivered_at) for reprint dedupe
templates/                 Typst ticket template (one file, themed by --input theme=)
web/                       Tailwind v4 + esbuild + connect-web frontend
```
