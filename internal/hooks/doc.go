// Package hooks serves agent-runtime hook connections in the daemon.
//
// A hook is a short-lived process (`aim hook <runtime>`) that a coding agent
// runs on lifecycle events. It connects with a preamble of kind "hook", sends
// one wire.HookRequest and reads one wire.HookReply. The server joins the hook
// to the same agent's MCP connection (by session id, else by agent pid on the
// machine, else by the persisted sign-on) to learn its identity, and replies
// with Markdown context: who the agent is, new mail since the last nudge, or a
// reminder to sign on.
//
// A watch request blocks until new mail arrives for the agent and then
// replies, which lets runtimes that support it wake an idle session. Each
// agent has at most one watch; a newer one supersedes the older.
//
// An approve request carries a permission prompt. With remote approvals
// enabled it waits, listed by Approvals, until the operator calls Decide,
// the timeout passes or the hook goes away; only a decision answers the
// prompt, otherwise the agent prompts in its terminal as usual.
//
// The server also records which subagent issued an upcoming MCP tool call
// (from a pre-tool hook's tool-use id) so the MCP server can attribute the
// call without the agent passing `subagent` itself.
package hooks
