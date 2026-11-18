//! The state Bode shows, as one value.

use crate::balloon::Balloon;

/// Everything Bode knows at one moment, published after each refresh.
#[derive(Debug, Clone, PartialEq, Default)]
pub struct Snapshot {
    pub balloons: Vec<Balloon>,
    pub frozen: bool,
    pub status: DomjudgeStatus,
}

/// How the last attempt to reach DOMjudge went.
#[derive(Debug, Clone, PartialEq, Default)]
pub enum DomjudgeStatus {
    /// No DOMjudge connection stored yet.
    #[default]
    Unconfigured,
    Ok,
    /// The error and its causes, readable for the admin.
    Error(String),
}
