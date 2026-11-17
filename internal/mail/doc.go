// Package mail implements the AIM mail service: registering identities,
// sending and threading messages, reading mailboxes, tracking per-delivery
// read and archive state, and full-text search.
//
// The service enforces the business rules on top of internal/store. Reads are
// anonymous: an address names a subtree (exact at its own level, recursive
// below) and needs no sign-on. Mutations require a signed-on Caller and only
// ever touch deliveries owned by the caller's identity mailbox or, when a
// subagent name is given, that subagent's mailbox under the caller's identity.
//
// Presence is consumed through the narrow Presence interface so the package
// does not depend on the presence registry. Error messages are written to be
// read by AI agents: they say what is wrong and what to do next.
package mail
