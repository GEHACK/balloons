# CLAUDE.md

Guidance for Claude Code (claude.ai/code) in this repository.

@README.md

## Context

- The Go version on `main` serves only as a feature list: it says what Bode must do, not how. Don't carry over its design or naming. Read it with `git show main:<path>` rather than reintroducing it here.
- `bode-core` is compiled to wasm as well as natively: keep it free of IO and of server-only dependencies.
- In `crates/app`, server-only dependencies are `optional` and enabled by the `server` feature; code using them sits behind `#[cfg(feature = "server")]`.
