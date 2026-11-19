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
            self.sender.send_if_modified(|current| current.merge(new));
        }
    }

    pub async fn mark_delivered(&self, balloon_id: u32) -> anyhow::Result<()> {
        let client = client(&self.store)
            .await?
            .context("DOMjudge connection not configured")?;
        client
            .mark_balloon_delivered(balloon_id)
            .await
            .with_context(|| format!("marking balloon {balloon_id} delivered"))?;
        self.trigger_refresh();
        Ok(())
    }
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

/// A client for the stored connection, `None` if there is none.
async fn client(store: &Store) -> anyhow::Result<Option<Client>> {
    let Some(config) = store
        .domjudge_config()
        .await
        .context("reading DOMjudge connection")?
    else {
        return Ok(None);
    };
    let client = Client::new(config).context("creating DOMjudge client")?;
    Ok(Some(client))
}

async fn fetch(store: &Store) -> anyhow::Result<Option<Snapshot>> {
    let Some(client) = client(store).await? else {
        return Ok(None);
    };
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
