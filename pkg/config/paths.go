package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir is git-agent's per-user directory ($XDG_CONFIG_HOME/git-agent, usually
// ~/.config/git-agent). It holds the history file and audit trail, and is the
// last place config.yaml is looked for.
func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ".git-agent" // ponytail: no HOME at all; keep working from cwd
	}
	return filepath.Join(base, "git-agent")
}

// HistoryFile is where the REPL keeps command history.
func HistoryFile() string { return filepath.Join(Dir(), "history") }

// AuditDir returns the audit trail directory. An audit DB left in the old
// cwd-relative location is moved here once; if it cannot be moved, the old
// location keeps being used so no history is lost.
func AuditDir() (dir, notice string) {
	dir = filepath.Join(Dir(), "audit")
	const legacy = ".git-agent/audit"
	legacyDB := filepath.Join(legacy, "audit.db")
	if _, err := os.Stat(legacyDB); err != nil {
		return dir, ""
	}
	if abs, _ := filepath.Abs(legacy); abs == dir {
		return dir, ""
	}
	if _, err := os.Stat(filepath.Join(dir, "audit.db")); err == nil {
		return dir, "" // already migrated from another checkout; leave the old one alone
	}
	if err := os.MkdirAll(dir, 0o700); err == nil {
		if err := os.Rename(legacyDB, filepath.Join(dir, "audit.db")); err == nil {
			return dir, fmt.Sprintf("Moved audit trail from %s to %s", legacy, dir)
		}
	}
	return legacy, ""
}
