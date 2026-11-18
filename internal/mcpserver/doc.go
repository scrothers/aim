// Package mcpserver exposes the mail service to AI agents over MCP.
//
// A server is built per connection so that every handler closes over that
// connection's presence entry; no session lookup is needed. New wires the
// feature packages together, one per MCP feature:
//
//   - tools: one <name>.go (types and handler) and <name>.md (description)
//     per tool.
//   - resources: one <name>.go and <name>.md per aim:// resource or template,
//     plus resource subscriptions.
//   - prompts: one <name>.md workflow script and <name>.go behavior per
//     user-invocable prompt.
//   - completions: argument completion for prompts and resource templates.
//
// They share per-connection state through package session. This package keeps
// the server instructions (instructions.md), the client-info and subagent
// attribution middleware, and the presence adapter for the mail service.
package mcpserver
