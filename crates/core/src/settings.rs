//! Settings an admin edits in the web UI.

use std::collections::HashSet;

#[derive(Debug, Clone, PartialEq)]
pub struct Settings {
    /// IANA name such as `"Europe/Amsterdam"`. `None` means server time.
    pub timezone: Option<String>,
    pub loom_url: String,
}

/// DOMjudge groups that get special treatment. A group can be in both sets.
#[derive(Debug, Clone, PartialEq, Default)]
pub struct GroupFilters {
    /// Balloons of these groups aren't shown at all.
    pub hidden: HashSet<String>,
    /// Solves of these groups never count as first solve.
    pub no_first_solve: HashSet<String>,
}
