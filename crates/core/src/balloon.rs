//! Balloons as Bode shows them, and how they're made from DOMjudge's.

use crate::domjudge;
use crate::settings::GroupFilters;
use std::collections::{HashMap, HashSet};
use std::hash::Hash;

#[derive(Debug, Clone, PartialEq)]
pub struct Balloon {
    pub id: u32,
    pub problem_label: String,
    pub problem_rgb: String,
    pub team_id: String,
    /// Without DOMjudge's `"<team id>: "` prefix.
    pub team_name: String,
    pub done: bool,
    /// Always `false` for now, see "First-solve race" in the README.
    pub first_solve: bool,
}

/// Drops balloons of hidden groups. A team missing from `teams` stays
/// visible, so an incomplete team list never hides a balloon.
pub fn from_domjudge(
    balloons: Vec<domjudge::Balloon>,
    teams: &[domjudge::Team],
    filters: &GroupFilters,
) -> Vec<Balloon> {
    let team_groups = team_groups(teams);
    balloons
        .into_iter()
        .filter(|balloon| !is_hidden(balloon, &team_groups, filters))
        .map(to_balloon)
        .collect()
}

fn team_groups(teams: &[domjudge::Team]) -> HashMap<&str, &[String]> {
    teams
        .iter()
        .map(|team| (team.id.as_str(), team.group_ids.as_slice()))
        .collect()
}

fn is_hidden(
    balloon: &domjudge::Balloon,
    team_groups: &HashMap<&str, &[String]>,
    filters: &GroupFilters,
) -> bool {
    let groups = team_groups
        .get(balloon.team_id.as_str())
        .copied()
        .unwrap_or(&[]);
    any_in(groups, &filters.hidden)
}

fn to_balloon(balloon: domjudge::Balloon) -> Balloon {
    Balloon {
        id: balloon.balloon_id,
        problem_label: balloon.contest_problem.label,
        problem_rgb: balloon.contest_problem.rgb,
        team_id: balloon.team_id,
        team_name: strip_label(&balloon.team).to_string(),
        done: balloon.done,
        first_solve: first_solve(),
    }
}

fn strip_label(team: &str) -> &str {
    match team.split_once(":") {
        Some((_, name)) => name.trim(),
        None => team,
    }
}

fn any_in<T: Hash + Eq>(items: &[T], set: &HashSet<T>) -> bool {
    items.iter().any(|item| set.contains(item))
}

fn first_solve() -> bool {
    false
}
