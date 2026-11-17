// Package daemon runs the AIM server: it listens on a unix socket and TCP,
// performs the connection preamble handshake, and serves one MCP session per
// connection, alongside the HTTP API.
package daemon
