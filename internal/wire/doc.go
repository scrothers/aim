// Package wire implements the connection preamble shared by the aim shim and
// daemon. Every connection starts with one JSON line describing the calling
// session, followed by newline-delimited MCP JSON-RPC.
package wire
