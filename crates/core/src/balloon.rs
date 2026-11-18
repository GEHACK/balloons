use crate::domjudge;
use std::collections::{HashMap, HashSet};
use std::hash::Hash;

#[derive(Debug, Clone, PartialEq, Default)]
pub struct GroupFilters {
    pub hidden: HashSet<String>,
    pub no_first_solve: HashSet<String>,
}

#[derive(Debug, Clone, PartialEq)]
pub struct Balloon {
    pub id: u32,
    pub problem_label: String,
    pub problem_rgb: String,
    pub team_id: String,
    pub team_name: String,
    pub done: bool,
    pub first_solve: bool,
}

pub fn from_domjudge(
    balloons_from_dj: Vec<domjudge::Balloon>,
    teams: &[domjudge::Team],
    filter: &GroupFilters,
) -> Vec<Balloon> {
    let mut team_to_groups: HashMap<&str, &[String]> = HashMap::new();
    for t in teams {
        team_to_groups.insert(&t.id, &t.group_ids);
    }
    let visible: Vec<domjudge::Balloon> = balloons_from_dj
        .into_iter()
        .filter(|b| {
            let groups = team_to_groups
                .get(b.team_id.as_str())
                .copied()
                .unwrap_or(&[]);
            !any_in(groups, &filter.hidden)
        })
        .collect();
    visible
        .into_iter()
        .map(|balloon| Balloon {
            id: balloon.balloon_id,
            problem_label: balloon.contest_problem.label,
            problem_rgb: balloon.contest_problem.rgb,
            team_id: balloon.team_id,
            team_name: strip_label(&balloon.team).to_string(),
            done: balloon.done,
            first_solve: first_solve(),
        })
        .collect()
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
