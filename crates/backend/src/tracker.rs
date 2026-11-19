//! Keeps Bode's view of the contest in sync with DOMjudge.

use crate::{domjudge::Client, store::Store};
use anyhow::Context;
use bode_core::{
    balloon::from_domjudge,
    snapshot::{DomjudgeStatus, Snapshot},
};
use std::time::Duration;
use tokio::sync::{Notify, watch};

pub struct Tracker {
    store: Store,
    sender: watch::Sender<Snapshot>,
    refresh_requested: Notify,
}

impl Tracker {
    pub fn new(store: Store) -> Self {
        Tracker {
            store,
            sender: watch::Sender::new(Snapshot::default()),
            refresh_requested: Notify::new(),
        }
    }

    pub fn subscribe(&self) -> watch::Receiver<Snapshot> {
        self.sender.subscribe()
    }

    pub fn trigger_refresh(&self) {
        self.refresh_requested.notify_one();
    }

    pub async fn run(&self) -> ! {
        let mut interval = tokio::time::interval(Duration::from_secs(5));
        loop {
            tokio::select! {
                _ = interval.tick() => {}
                _ = self.refresh_requested.notified() => {}
            };
            let new = refresh(&self.store).await;
            self.sender.send_if_modified(|current| merge(current, new));
        }
    }
}

fn merge(current: &mut Snapshot, new: Snapshot) -> bool {
    let wanted = match new.status {
        DomjudgeStatus::Error(e) => Snapshot {
            status: DomjudgeStatus::Error(e),
            ..current.clone()
        },
        _ => new,
    };
    if *current == wanted {
        return false;
    }
    *current = wanted;
    true
}

/// Fetches everything once. Never fails: every outcome is a snapshot, and on
/// an error it has no balloons (keeping the last ones is up to the caller).
pub async fn refresh(store: &Store) -> Snapshot {
    match fetch(store).await {
        Ok(Some(snapshot)) => snapshot,
        Ok(None) => Snapshot {
            status: DomjudgeStatus::Unconfigured,
            ..Default::default()
        },
        Err(e) => Snapshot {
            status: DomjudgeStatus::Error(format!("{e:#}")),
            ..Default::default()
        },
    }
}

async fn fetch(store: &Store) -> anyhow::Result<Option<Snapshot>> {
    let config = match store
        .domjudge_config()
        .await
        .context("reading DOMjudge connection")?
    {
        Some(c) => c,
        None => return Ok(None),
    };
    let client = Client::new(config).context("creating DOMjudge client")?;
    let balloons_from_dj = client.fetch_balloons().await.context("fetching balloons")?;
    let teams = client.fetch_teams().await.context("fetching teams")?;
    let filters = store
        .group_filters()
        .await
        .context("reading group filters")?;
    let balloons = from_domjudge(balloons_from_dj, &teams, &filters);
    let frozen = client
        .fetch_contest_state()
        .await
        .context("fetching contest state")?
        .is_frozen();
    Ok(Some(Snapshot {
        balloons,
        frozen,
        status: DomjudgeStatus::Ok,
    }))
}

#[cfg(test)]
mod tests {
    use super::*;
    use bode_core::balloon::Balloon;

    fn balloon(id: u32) -> Balloon {
        Balloon {
            id,
            problem_label: "A".to_string(),
            problem_rgb: "#ff0000".to_string(),
            team_id: "exteam".to_string(),
            team_name: "Example team".to_string(),
            done: false,
            first_solve: false,
        }
    }

    fn ok(balloons: Vec<Balloon>, frozen: bool) -> Snapshot {
        Snapshot {
            balloons,
            frozen,
            status: DomjudgeStatus::Ok,
        }
    }

    fn error(message: &str) -> Snapshot {
        Snapshot {
            status: DomjudgeStatus::Error(message.to_string()),
            ..Default::default()
        }
    }

    #[test]
    fn ok_replaces_everything() {
        let mut current = ok(vec![balloon(1)], false);
        let new = ok(vec![balloon(1), balloon(2)], true);

        assert!(merge(&mut current, new.clone()));
        assert_eq!(current, new);
    }

    #[test]
    fn same_snapshot_is_no_change() {
        let mut current = ok(vec![balloon(1)], false);
        let same = current.clone();

        assert!(!merge(&mut current, same));
    }

    #[test]
    fn error_keeps_balloons_and_frozen() {
        let mut current = ok(vec![balloon(1)], true);

        assert!(merge(&mut current, error("connection refused")));
        assert_eq!(current.balloons, vec![balloon(1)]);
        assert!(current.frozen);
        assert_eq!(
            current.status,
            DomjudgeStatus::Error("connection refused".to_string())
        );
    }

    #[test]
    fn same_error_is_no_change() {
        let mut current = ok(vec![balloon(1)], false);
        merge(&mut current, error("connection refused"));

        assert!(!merge(&mut current, error("connection refused")));
    }

    #[test]
    fn unconfigured_clears_balloons() {
        let mut current = ok(vec![balloon(1)], true);
        let new = Snapshot {
            status: DomjudgeStatus::Unconfigured,
            ..Default::default()
        };

        assert!(merge(&mut current, new.clone()));
        assert_eq!(current, new);
    }
}
