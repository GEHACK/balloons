mod common;

use bode_backend::{
    domjudge::Config,
    store::Store,
    tracker::{Tracker, refresh},
};
use bode_core::snapshot::DomjudgeStatus;
use common::fresh_store;
use std::{sync::Arc, time::Duration};
use tokio::time::timeout;

/// Nothing listens on port 1, so the connection is refused right away.
async fn configure_unreachable(store: &Store) {
    store
        .set_domjudge_config(&Config {
            base_url: "http://127.0.0.1:1".to_string(),
            user: "balloon".to_string(),
            password: "secret".to_string(),
            contest_id: "demo".to_string(),
        })
        .await
        .unwrap();
}

#[tokio::test]
async fn unconfigured_store_gives_unconfigured() {
    let (store, _dir) = fresh_store().await;
    let snapshot = refresh(&store).await;
    assert_eq!(snapshot.status, DomjudgeStatus::Unconfigured);
    assert!(snapshot.balloons.is_empty());
}

#[tokio::test]
async fn unreachable_domjudge_gives_error() {
    let (store, _dir) = fresh_store().await;
    configure_unreachable(&store).await;

    let snapshot = refresh(&store).await;
    assert!(
        matches!(snapshot.status, DomjudgeStatus::Error(_)),
        "expected an error, got {:?}",
        snapshot.status
    );
}

#[tokio::test]
async fn fresh_tracker_is_unconfigured() {
    let (store, _dir) = fresh_store().await;
    let tracker = Tracker::new(store);

    assert_eq!(
        tracker.subscribe().borrow().status,
        DomjudgeStatus::Unconfigured
    );
}

#[tokio::test]
async fn running_tracker_publishes_error_for_unreachable_domjudge() {
    let (store, _dir) = fresh_store().await;
    configure_unreachable(&store).await;
    let tracker = Arc::new(Tracker::new(store));
    let mut rx = tracker.subscribe();

    let runner = Arc::clone(&tracker);
    tokio::spawn(async move { runner.run().await });

    timeout(Duration::from_secs(10), rx.changed())
        .await
        .expect("no snapshot within 10 seconds")
        .unwrap();
    let status = rx.borrow_and_update().status.clone();
    assert!(
        matches!(status, DomjudgeStatus::Error(_)),
        "expected an error, got {status:?}"
    );
}
