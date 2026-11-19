//! Runs a tracker against a live DOMjudge and prints each change.
//!
//! Reads the connection from `DJ_URL`, `DJ_USER`, `DJ_PASSWORD` and
//! `DJ_CONTEST`. Try `just dj-solve fltcmp`, `just dj-down` and `just dj-up`
//! while it runs.

use anyhow::Context;
use bode_backend::{domjudge::Config, store::Store, tracker::Tracker};
use bode_core::snapshot::Snapshot;
use std::{env, sync::Arc};
use tempfile::tempdir;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let tempdir = tempdir().context("creating tempdir")?;
    let store = Store::open(tempdir.path().join("db.db"))
        .await
        .context("opening store")?;
    store.set_domjudge_config(&load_env()?).await?;

    let tracker = Arc::new(Tracker::new(store));
    // Subscribe before the tracker starts, so the first snapshot isn't missed.
    let mut rx = tracker.subscribe();
    let runner = Arc::clone(&tracker);
    tokio::spawn(async move { runner.run().await });

    let mut prev = Snapshot::default();
    while rx.changed().await.is_ok() {
        let snapshot = rx.borrow_and_update().clone();
        if snapshot.status != prev.status {
            println!("status: {:?}", snapshot.status);
        }
        if snapshot.frozen != prev.frozen {
            println!("frozen: {}", snapshot.frozen);
        }
        if snapshot.balloons != prev.balloons {
            println!("balloons: {}", snapshot.balloons.len());
        }
        prev = snapshot;
    }
    Ok(())
}

fn load_env() -> anyhow::Result<Config> {
    Ok(Config {
        base_url: env::var("DJ_URL").context("DJ_URL not set")?,
        user: env::var("DJ_USER").context("DJ_USER not set")?,
        password: env::var("DJ_PASSWORD").context("DJ_PASSWORD not set")?,
        contest_id: env::var("DJ_CONTEST").context("DJ_CONTEST not set")?,
    })
}
