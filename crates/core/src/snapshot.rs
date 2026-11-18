use crate::balloon::Balloon;

#[derive(Debug, Clone, PartialEq, Default)]
pub struct Snapshot {
    pub balloons: Vec<Balloon>,
    pub frozen: bool,
    pub status: DomjudgeStatus,
}

#[derive(Debug, Clone, PartialEq, Default)]
pub enum DomjudgeStatus {
    #[default]
    Unconfigured,
    Ok,
    Error(String),
}
