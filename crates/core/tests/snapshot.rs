use bode_core::balloon::Balloon;
use bode_core::snapshot::{DomjudgeStatus, Snapshot};

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

    assert!(current.merge(new.clone()));
    assert_eq!(current, new);
}

#[test]
fn same_snapshot_is_no_change() {
    let mut current = ok(vec![balloon(1)], false);
    let same = current.clone();

    assert!(!current.merge(same));
}

#[test]
fn error_keeps_balloons_and_frozen() {
    let mut current = ok(vec![balloon(1)], true);

    assert!(current.merge(error("connection refused")));
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
    current.merge(error("connection refused"));

    assert!(!current.merge(error("connection refused")));
}

#[test]
fn unconfigured_clears_balloons() {
    let mut current = ok(vec![balloon(1)], true);
    let new = Snapshot {
        status: DomjudgeStatus::Unconfigured,
        ..Default::default()
    };

    assert!(current.merge(new.clone()));
    assert_eq!(current, new);
}
