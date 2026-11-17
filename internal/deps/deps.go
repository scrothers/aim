// Package deps pins the planned direct dependencies so go.mod stays stable
// while packages are built in parallel. Remove it once real packages import
// each module.
package deps

import (
	// Pinned planned dependencies; see package comment.
	_ "github.com/danielgtaylor/huma/v2"
	// Pinned planned dependency.
	_ "github.com/go-chi/chi/v5"
	// Pinned planned dependency.
	_ "github.com/golang-migrate/migrate/v4"
	// Pinned planned dependency.
	_ "github.com/modelcontextprotocol/go-sdk/mcp"
	// Pinned planned dependency.
	_ "github.com/oklog/ulid/v2"
	// Pinned planned dependency.
	_ "github.com/spf13/cobra"
	// Pinned planned dependency.
	_ "modernc.org/sqlite"
)
