// Package mcpserver exposes the mail service to AI agents over MCP.
//
// A server is built per connection so that every handler closes over that
// connection's presence entry; no session lookup is needed. New wires the
// feature packages together, one per MCP feature:
//
//   - tools: one <name>.go (types and handler) and markdown/<name>.md
//     (description) per tool.
//   - resources: one <name>.go and markdown/<name>.md per aim:// resource or
//     template, plus resource subscriptions.
//   - prompts: one markdown/<name>.md workflow script and <name>.go behavior
//     per user-invocable prompt.
//   - completions: argument completion for prompts and resource templates.
//
// Each feature package keeps its Markdown in a markdown/ directory, apart from
// its Go code, and embeds it.
//
// They share per-connection state through package session. This package keeps
// the server instructions (instructions.md), the client-info and subagent
// attribution middleware, and the presence adapter for the mail service.
package mcpserver
