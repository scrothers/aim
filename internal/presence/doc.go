// Package presence tracks the live daemon connections, one per Claude Code
// session, and which identity each connection has signed on as.
//
// A connection that is open means its session is online. A connection may
// sign on as one identity address, and several connections may sign on as the
// same identity. The registry is in-memory only and is safe for concurrent use.
package presence
