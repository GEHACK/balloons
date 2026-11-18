use std::collections::HashSet;

use bode_core::balloon::{self, Balloon, GroupFilters};
use bode_core::domjudge;

fn dj_balloon(id: u32, team_id: &str, team: &str, problem: &str) -> domjudge::Balloon {
    domjudge::Balloon {
        balloon_id: id,
        time: format!("17315001{id:02}.000000000"),
        contest_problem: domjudge::ContestProblem {
            id: problem.to_lowercase(),
            label: problem.to_string(),
            rgb: "#ff0000".to_string(),
        },
        team: team.to_string(),
        team_id: team_id.to_string(),
        done: false,
    }
}

fn dj_team(id: &str, groups: &[&str]) -> domjudge::Team {
    domjudge::Team {
        id: id.to_string(),
        group_ids: groups.iter().map(|g| g.to_string()).collect(),
    }
}

fn filters(hidden: &[&str]) -> GroupFilters {
    GroupFilters {
        hidden: hidden.iter().map(|g| g.to_string()).collect(),
        no_first_solve: HashSet::new(),
    }
}

fn ids(balloons: &[Balloon]) -> Vec<u32> {
    balloons.iter().map(|b| b.id).collect()
}

#[test]
fn converts_all_fields() {
    let mut b = dj_balloon(7, "41", "41: Team DOMjudge", "B");
    b.done = true;
    let teams = [dj_team("41", &["3"])];

    let out = balloon::from_domjudge(vec![b], &teams, &filters(&[]));

    assert_eq!(
        out,
        [Balloon {
            id: 7,
            problem_label: "B".to_string(),
            problem_rgb: "#ff0000".to_string(),
            team_id: "41".to_string(),
            team_name: "Team DOMjudge".to_string(),
            done: true,
            first_solve: false,
        }]
    );
}

#[test]
fn team_name_strips_prefix() {
    let cases = [
        ("41: Team DOMjudge", "Team DOMjudge"),
        ("41: Foo: Bar", "Foo: Bar"),
        ("Team DOMjudge", "Team DOMjudge"),
    ];
    for (team, expected) in cases {
        let out = balloon::from_domjudge(
            vec![dj_balloon(1, "41", team, "A")],
            &[dj_team("41", &[])],
            &filters(&[]),
        );
        assert_eq!(out[0].team_name, expected, "team string: {team:?}");
    }
}

#[test]
fn hidden_team_disappears() {
    let balloons = vec![
        dj_balloon(1, "41", "41: Visible", "A"),
        dj_balloon(2, "42", "42: Hidden", "A"),
    ];
    let teams = [dj_team("41", &["3"]), dj_team("42", &["9"])];

    let out = balloon::from_domjudge(balloons, &teams, &filters(&["9"]));

    assert_eq!(ids(&out), [1]);
}

#[test]
fn any_matching_group_hides() {
    let balloons = vec![dj_balloon(1, "41", "41: Multi", "A")];
    let teams = [dj_team("41", &["3", "9", "12"])];

    let out = balloon::from_domjudge(balloons, &teams, &filters(&["9"]));

    assert!(out.is_empty());
}

#[test]
fn team_missing_from_teams_stays_visible() {
    let balloons = vec![dj_balloon(1, "99", "99: Unknown", "A")];
    let teams = [dj_team("41", &["9"])];

    let out = balloon::from_domjudge(balloons, &teams, &filters(&["9"]));

    assert_eq!(ids(&out), [1]);
}

#[test]
fn first_solve_is_always_false_for_now() {
    let balloons = vec![
        dj_balloon(1, "41", "41: A", "A"),
        dj_balloon(2, "42", "42: B", "A"),
    ];
    let teams = [dj_team("41", &[]), dj_team("42", &[])];

    let out = balloon::from_domjudge(balloons, &teams, &filters(&[]));

    assert!(out.iter().all(|b| !b.first_solve));
}
