// Package shim implements the `aim mcp` stdio shim. Claude Code launches it as
// a stdio MCP server; it dials the aim daemon, sends a session preamble and
// then proxies newline-delimited JSON-RPC in both directions. If the daemon
// connection drops the shim redials, re-sends the preamble with resume set,
// replays the cached initialize handshake and delivers requests queued in the
// meantime, so a daemon restart is transparent to the client. Requests that
// cannot be delivered in time fail with a JSON-RPC error rather than hanging.
// Stdout carries MCP messages only; diagnostics go to the configured logger.
package shim
