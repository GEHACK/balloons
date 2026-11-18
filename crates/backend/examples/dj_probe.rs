use anyhow::Context;
use bode_backend::domjudge::{Client, Config};
use bode_core::balloon::{GroupFilters, from_domjudge};
use std::collections::HashSet;
use std::env;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let config = load_env()?;
    let client = Client::new(config)?;
    let teams = client.fetch_teams().await?;
    for t in &teams {
        println!("{}, {:?}", t.id, t.group_ids);
    }

    let balloons = client.fetch_balloons().await?;
    for b in &balloons {
        println!("{}: {}", b.team, b.balloon_id);
    }

    let probs = client.fetch_problems().await?;
    for p in probs {
        println!("{}", p.id);
    }

    let state = client.fetch_contest_state().await?;
    println!("{}", state.is_frozen());

    let mut filters = GroupFilters {
        hidden: HashSet::new(),
        no_first_solve: HashSet::new(),
    };

    let f = from_domjudge(balloons, &teams, &filters);

    for b in f {
        println!("{}: {}", b.team_name, b.id);
    }

    filters.hidden.insert("system".to_string());

    let balloons = client.fetch_balloons().await?;

    let f = from_domjudge(balloons, &teams, &filters);

    println!("No more system");
    for b in f {
        println!("{}: {}", b.team_name, b.id);
    }
    Ok(())
}

fn load_env() -> Result<Config, anyhow::Error> {
    Ok(Config {
        base_url: env::var("DJ_URL").context("DOMjudge URL not set")?,
        user: env::var("DJ_USER").context("DOMjudge user not set")?,
        password: env::var("DJ_PASSWORD").context("DOMjudge user password not set")?,
        contest_id: env::var("DJ_CONTEST").context("DOMjudge contest not set")?,
    })
}
