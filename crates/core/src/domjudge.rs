//! DOMjudge API v4 wire types, with only the fields Bode uses.

use serde::Deserialize;

/// An entry of `/balloons`.
#[derive(Debug, Deserialize)]
pub struct Balloon {
    #[serde(rename = "balloonid")]
    pub balloon_id: u32,
    /// Submit time as decimal unix seconds, kept as text.
    pub time: String,
    #[serde(rename = "contestproblem")]
    pub contest_problem: ContestProblem,
    /// `"<team id>: <team name>"`.
    pub team: String,
    /// External team id, the same as in `/teams`.
    #[serde(rename = "teamid")]
    pub team_id: String,
    pub done: bool,
}

#[derive(Debug, Deserialize)]
pub struct ContestProblem {
    pub id: String,
    pub label: String,
    /// `#rrggbb`.
    pub rgb: String,
}

#[derive(Debug, Deserialize)]
pub struct Team {
    pub id: String,
    pub group_ids: Vec<String>,
}

#[derive(Debug, Deserialize)]
pub struct ContestState {
    pub frozen: Option<String>,
    pub thawed: Option<String>,
}

impl ContestState {
    /// Frozen and not yet thawed.
    pub fn is_frozen(&self) -> bool {
        self.frozen.is_some() && self.thawed.is_none()
    }
}
