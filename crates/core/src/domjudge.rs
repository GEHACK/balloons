use serde::Deserialize;

#[derive(Debug, Deserialize)]
pub struct Balloon {
    #[serde(rename = "balloonid")]
    pub balloon_id: u32,
    pub time: String,
    #[serde(rename = "contestproblem")]
    pub contest_problem: ContestProblem,
    pub team: String,
    #[serde(rename = "teamid")]
    pub team_id: String,
    pub done: bool,
}

#[derive(Debug, Deserialize)]
pub struct ContestProblem {
    pub id: String,
    pub label: String,
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
    pub fn is_frozen(&self) -> bool {
        self.frozen.is_some() && self.thawed.is_none()
    }
}
