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

impl Snapshot {
    /// Takes in a freshly fetched snapshot and reports whether anything
    /// changed. On an error the balloons and `frozen` are kept, so a DOMjudge
    /// hiccup doesn't empty the screen.
    pub fn merge(&mut self, new: Snapshot) -> bool {
        let wanted = match new.status {
            DomjudgeStatus::Error(e) => Snapshot {
                status: DomjudgeStatus::Error(e),
                ..self.clone()
            },
            _ => new,
        };
        if *self == wanted {
            return false;
        }
        *self = wanted;
        true
    }
}
