use bode_core::domjudge::{Balloon, ContestState, Team};

#[test]
fn balloon_deserializes() {
    let json = r##"{
      "balloonid": 41,
      "time": "1731500123.456789000",
      "problem": "B",
      "contestproblem": { "id": "fltcmp", "label": "B", "rgb": "#ff0000", "color": "red" },
      "team": "t41: Team DOMjudge",
      "teamid": "41",
      "location": "Room 2, seat 14",
      "affiliation": "TU Eindhoven",
      "total": [],
      "done": false
    }"##;
    let b: Balloon = serde_json::from_str(json).unwrap();
    assert_eq!(b.balloon_id, 41)
}

#[test]
fn groups_parsing() {
    let json = r#"{ "id": "41", "group_ids": ["3", "7"], "name": "Team DOMjudge" }"#;
    let t: Team = serde_json::from_str(json).unwrap();
    assert!(t.group_ids.iter().eq(["3", "7"].iter()))
}

#[test]
fn contest_parsing() {
    let test_cases = [
        (r#"{ "frozen": null, "thawed": null }"#, false),
        (
            r#"{ "frozen": "2026-11-13T15:00:00.000+01:00", "thawed": null }"#,
            true,
        ),
        (
            r#"{ "frozen": "2026-11-13T15:00:00.000+01:00", "thawed": "2026-11-13T17:30:00.000+01:00" }"#,
            false,
        ),
    ];
    for (json, res) in test_cases {
        contest_state_check(json, res)
    }
}

fn contest_state_check(json: &str, expected_state: bool) {
    let s: ContestState = serde_json::from_str(json).unwrap();
    assert_eq!(s.is_frozen(), expected_state, "state: {json}")
}
