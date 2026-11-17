// Package address parses, validates and compares AIM mailbox addresses.
//
// An address has the form
//
//	[subagent@][identity@]host/org[/repo]
//
// With no '@' it names a namespace mailbox (an org or a repo), with one it
// names an identity, and with two it names a subagent of an identity.
// Addresses are normalized to lower case with surrounding spaces trimmed.
//
// Reading an address is exact at its own level and recursive below it; see
// [Address.Contains] and [Address.Scope].
package address
