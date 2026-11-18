mod common;

use std::collections::HashSet;

use bode_backend::domjudge::Config;
use bode_core::settings::{GroupFilters, Settings};
use common::fresh_store;

fn sample_config() -> Config {
    Config {
        base_url: "https://dj".to_string(),
        user: "bartjan".to_string(),
        password: "gehack".to_string(),
        contest_id: "euc2027".to_string(),
    }
}

#[tokio::test]
async fn fresh_store_is_empty() {
    let (store, _dir) = fresh_store().await;
    let config = store.domjudge_config().await.unwrap();
    assert_eq!(config, None)
}

#[tokio::test]
async fn set_fresh_config() {
    let (store, _dir) = fresh_store().await;
    let conf = sample_config();
    store.set_domjudge_config(&conf).await.unwrap();
    let conf_in_db = store.domjudge_config().await.unwrap();
    assert_eq!(Some(conf), conf_in_db)
}

#[tokio::test]
async fn changing_config() {
    let (store, _dir) = fresh_store().await;
    let conf = sample_config();
    let conf2 = Config {
        base_url: "https://gehack".to_string(),
        user: "luuk".to_string(),
        password: "loom".to_string(),
        contest_id: "bapc2026".to_string(),
    };
    store.set_domjudge_config(&conf).await.unwrap();
    store.set_domjudge_config(&conf2).await.unwrap();
    let conf_in_db = store.domjudge_config().await.unwrap();
    assert_eq!(Some(conf2), conf_in_db)
}

#[tokio::test]
async fn fresh_settings_are_defaults() {
    let (store, _dir) = fresh_store().await;
    let settings = store.settings().await.unwrap();
    assert_eq!(
        settings,
        Settings {
            timezone: None,
            loom_url: "https://loom.gehack.nl".to_string(),
        }
    )
}

#[tokio::test]
async fn changing_settings() {
    let (store, _dir) = fresh_store().await;
    let settings = Settings {
        timezone: Some("Europe/Amsterdam".to_string()),
        loom_url: "https://loom.example".to_string(),
    };
    store.set_settings(&settings).await.unwrap();
    assert_eq!(store.settings().await.unwrap(), settings);

    let cleared = Settings {
        timezone: None,
        ..settings
    };
    store.set_settings(&cleared).await.unwrap();
    assert_eq!(store.settings().await.unwrap(), cleared)
}

fn groups(ids: &[&str]) -> HashSet<String> {
    ids.iter().map(|id| id.to_string()).collect()
}

#[tokio::test]
async fn fresh_group_filters_are_empty() {
    let (store, _dir) = fresh_store().await;
    assert_eq!(
        store.group_filters().await.unwrap(),
        GroupFilters::default()
    )
}

#[tokio::test]
async fn group_filters_round_trip() {
    let (store, _dir) = fresh_store().await;
    // Group 9 is in both sets.
    let filters = GroupFilters {
        hidden: groups(&["3", "9"]),
        no_first_solve: groups(&["9", "12"]),
    };
    store.set_group_filters(&filters).await.unwrap();
    assert_eq!(store.group_filters().await.unwrap(), filters)
}

#[tokio::test]
async fn setting_fewer_group_filters_removes_the_rest() {
    let (store, _dir) = fresh_store().await;
    store
        .set_group_filters(&GroupFilters {
            hidden: groups(&["3", "9"]),
            no_first_solve: groups(&["12"]),
        })
        .await
        .unwrap();

    let fewer = GroupFilters {
        hidden: groups(&["9"]),
        no_first_solve: groups(&[]),
    };
    store.set_group_filters(&fewer).await.unwrap();
    assert_eq!(store.group_filters().await.unwrap(), fewer)
}
