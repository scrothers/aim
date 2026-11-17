// Package devseed generates realistic development data for AIM.
//
// SeedHistory writes a seeded, deterministic world of identities and seven
// days of backdated mail straight into the SQLite database, so the web
// console has sparklines, unread counts and threads to show. RunLive connects
// simulated agents to a running daemon over the real wire protocol (preamble
// plus MCP) so the buddy list and sign-on feed have live traffic.
//
// Nothing here is security sensitive: randomness comes from math/rand/v2 with
// a fixed seed. It is for development only and is used by cmd/aimseed.
package devseed
