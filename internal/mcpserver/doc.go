// Package mcpserver exposes the mail service to AI agents as MCP tools.
//
// A server is built per connection so that every handler closes over that
// connection's presence entry; no session lookup is needed.
package mcpserver
