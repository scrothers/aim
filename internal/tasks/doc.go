// Package tasks implements AIM's global task board: work items agents and the
// operator create, assign, claim and finish, linked to mail.
//
// # Statuses
//
//	not_started --claim--> claimed --start--> started --finish--> finished
//	     ^                    |                  |
//	     +------release-------+------------------+
//	claimed/started --(lease decayed)--> abandoned --claim--> claimed
//	finished/abandoned --reopen--> not_started
//
// start also claims an unclaimed task, so not_started and abandoned may go
// straight to started.
//
// # People and permissions
//
// People are canonical identity addresses, or "operator" for the human
// operator, who may perform every action. Agents act through a signed-on
// mail.Caller.
//
//   - create, comment, link: any signed-on caller.
//   - claim, start: the assignee while the task is assigned, otherwise anyone.
//     A claim records the caller's identity and session.
//   - finish: the claimant.
//   - release: the claimant or the creator.
//   - assign, unassign: the creator or the current assignee (to hand off).
//   - reopen: the creator.
//   - edit (title, body, priority, scope, due): the creator or the claimant.
//
// Every change appends an Event, so a task's history is complete.
//
// # Lease decay
//
// A claim is a lease kept alive by the claimant session's activity: its MCP
// tool calls, its hook events and its task updates all call Touch, which
// resets the task to HealthActive. Without activity, Health degrades from
// active to idle to stale (the claimant and creator are mailed once) and
// finally to abandoned, which Sweep stores as StatusAbandoned so anyone may
// claim the task again (an assignment lapses with the claim). Abandonment needs the stale period to pass and the
// claimant's session to have no live connection, or the hard maximum to
// pass regardless. Thresholds are set with WithDecay.
//
// # Notifications
//
// Assignment mails the assignee from the assigning identity. Decay notices
// are mailed from the claimant's identity with an "[AIM]" subject prefix and
// a body saying they were sent automatically. Every change broadcasts on
// Changed so subscribers re-read.
package tasks
