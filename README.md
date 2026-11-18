# Bode

Bode (Dutch for *messenger*) delivers balloons and print jobs at the GEHACK programming contest. It replaces DOMjudge's built-in balloon tool with what that tool lacks: a **first-solve** highlight, **printed tickets** with a QR code a runner scans to mark the balloon delivered, and tracking of team **print jobs** that go through cuproxy.

## Status

Rust rewrite in progress. The previous Go server and TypeScript frontend, named *balloons*, live on `main`. Its README (`git show main:README.md`) documents the behaviour to preserve and the integration lessons learned on contest hardware (DOMjudge, cuproxy, ESC/POS printers).

## Layout

A Cargo workspace:

| Crate | Package | Purpose |
| --- | --- | --- |
| `crates/core` | `bode-core` | Shared types and pure logic. Compiles for both the server and the browser (wasm), so no IO. |
| `crates/app` | `bode` | Dioxus fullstack app: web UI, server functions, server entry point. |

`templates/balloon.typ` is the Typst ticket template.

## Development

The Nix flake provides the dev shell. With direnv, `direnv allow` once and it loads automatically. Install `dx` (the Dioxus CLI) with Cargo rather than from nixpkgs: the nixpkgs build pins a `wasm-bindgen` that has to match the project's exactly.

```sh
cd crates/app && dx serve   # run the app with hot reload
cargo test                  # run all tests
```

A local DOMjudge with a judgehost runs in Docker (`dev/domjudge/`):

```sh
just dj-up                  # start it; prints the admin password
just dj-solve hello         # submit a correct solution, which creates a balloon
just dj-down                # stop it (just dj-reset also wipes its database)
```

It serves http://localhost:12345 with DOMjudge's demo contest (`demo`), problems `hello`, `fltcmp` and `boolfind`, and team `exteam`. It runs DOMjudge's `bleeding` images (built from DOMjudge main), the version Bode targets: release 9.0.1 still identifies teams in `/balloons` by internal number instead of the external id `/teams` uses. The images are amd64-only, so on Apple Silicon they run emulated and judging takes a few seconds.

## Open problems

### First-solve race

A team can submit first but be judged later. If team A submits at 10:00:00 and team B at 10:00:05, but B's judgement arrives first, Bode only sees B's balloon and flags it first-solve. When A's balloon appears with an earlier time, the flag moves to A, but B's ticket may already be printed and the balloon on its way.

DOMjudge has the same bug ([DOMjudge#2810](https://github.com/DOMjudge/domjudge/issues/2810)): it creates a balloon as soon as a submission is judged correct, so a fast later submission beat a slow earlier one to "first to solve". The maintainers called a correct fix non-trivial and removed first-to-solve from their balloon page (2025) rather than fix it, which is part of why Bode computes it itself.

Ideas, nothing decided:

- Hold the first-solve flag (and the ticket) until no submission for that problem with an earlier submit time is still pending, e.g. by checking DOMjudge's judging queue (`/submissions` without a final judgement). This is also the fix proposed in the DOMjudge issue.
- Also covers rejudges, which can change verdicts after the fact.
- A balloon's `time` is the submission's submit time (DOMjudge `BalloonService`, checked on main in Nov 2026), so ordering by `time` is right once every earlier submission is judged. The open question is only how to know none are still pending.

## Configuration

The DOMjudge connection, group filters, printers, contest timezone, scan URL and loom URL are set in the web UI and stored in SQLite. Only what's needed before the database opens comes from the environment (see `.env.example`):

| Variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `:8080` | Listen address |
| `STATE_DB` | `bode.db` | SQLite file holding ticket state, print jobs and settings |
