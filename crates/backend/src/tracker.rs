use crate::{domjudge::Client, store::Store};
use anyhow::Context;
use bode_core::{
    balloon::from_domjudge,
    snapshot::{DomjudgeStatus, Snapshot},
};

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
