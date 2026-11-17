// Package shim implements the `aim mcp` stdio shim. Claude Code launches it as
// a stdio MCP server; it dials the aim daemon, sends a session preamble and
// then pipes bytes in both directions.
package shim
