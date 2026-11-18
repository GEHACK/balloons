mod common;

use bode_backend::{domjudge::Config, tracker::refresh};
use bode_core::snapshot::DomjudgeStatus;
use common::fresh_store;

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
    // Nothing listens on port 1, so the connection is refused right away.
    store
        .set_domjudge_config(&Config {
            base_url: "http://127.0.0.1:1".to_string(),
            user: "balloon".to_string(),
            password: "secret".to_string(),
            contest_id: "demo".to_string(),
        })
        .await
        .unwrap();

    let snapshot = refresh(&store).await;
    assert!(
        matches!(snapshot.status, DomjudgeStatus::Error(_)),
        "expected an error, got {:?}",
        snapshot.status
    );
}
