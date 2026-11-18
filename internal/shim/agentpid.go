package shim

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// shells are process names skipped when looking for the agent: runtimes
// often start hooks (and sometimes MCP servers) through a shell.
var shells = map[string]bool{
	"sh": true, "bash": true, "dash": true, "zsh": true, "fish": true,
	"ksh": true, "env": true, "timeout": true,
}

// maxAncestors bounds the walk up the process tree.
const maxAncestors = 8

// ProcInfo returns the parent pid and command name of pid.
type ProcInfo func(pid int) (ppid int, name string, err error)

// AgentPID returns the pid of the nearest ancestor of the current process
// that isn't a shell: the agent runtime that spawned it, directly or through
// sh -c. It returns 0 when the process tree can't be read.
func AgentPID(info ProcInfo) int {
	if info == nil {
		info = procInfo
	}
	pid := os.Getppid()
	for range maxAncestors {
		if pid <= 1 {
			return 0
		}
		ppid, name, err := info(pid)
		if err != nil {
			// Without /proc (non-Linux), the parent is the best guess.
			return os.Getppid()
		}
		if !shells[name] {
			return pid
		}
		pid = ppid
	}
	return 0
}

// procInfo reads /proc/<pid>/stat. The command name is in parentheses and
// may itself contain spaces or parentheses, so fields are split after the
// last ')'.
func procInfo(pid int) (int, string, error) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, "", fmt.Errorf("shim: read proc stat: %w", err)
	}
	return parseStat(string(b))
}

func parseStat(s string) (int, string, error) {
	open := strings.IndexByte(s, '(')
	end := strings.LastIndexByte(s, ')')
	if open < 0 || end < open {
		return 0, "", fmt.Errorf("shim: malformed proc stat %q", s)
	}
	name := s[open+1 : end]
	fields := strings.Fields(s[end+1:])
	if len(fields) < 2 {
		return 0, "", fmt.Errorf("shim: malformed proc stat %q", s)
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, "", fmt.Errorf("shim: proc stat ppid: %w", err)
	}
	return ppid, name, nil
}

// sessionEnvs lists runtime session-id variables, most specific first.
var sessionEnvs = []string{"CLAUDE_CODE_SESSION_ID", "GEMINI_SESSION_ID", "GROK_SESSION_ID"}

// SessionID returns the first runtime session id found in the environment.
func SessionID(getenv func(string) string) string {
	for _, k := range sessionEnvs {
		if v := getenv(k); v != "" {
			return v
		}
	}
	return ""
}
