package harness

import (
	"errors"
	"path/filepath"
)

// ScopedPath joins userRel under Home for ScopeUser, or projRel under the
// project directory for ScopeProject.
func ScopedPath(opts InstallOptions, userRel, projRel string) (string, error) {
	if opts.Scope == ScopeProject {
		if opts.ProjectDir == "" {
			return "", errors.New("project scope needs a project directory")
		}
		return filepath.Join(opts.ProjectDir, projRel), nil
	}
	if opts.Home == "" {
		return "", errors.New("user scope needs a home directory")
	}
	return filepath.Join(opts.Home, userRel), nil
}

// ScopeFlag returns "user" or "project", as runtimes name their scopes.
func ScopeFlag(s Scope) string {
	if s == ScopeProject {
		return "project"
	}
	return "user"
}
