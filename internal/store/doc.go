// Package store is the SQLite persistence layer for aim.
//
// It owns schema migration (golang-migrate over embedded SQL files), a
// single-connection writer pool plus a read-only reader pool, the
// sqlc-generated queries in the db subpackage, and the hand-written FTS5
// search that sqlc cannot express.
package store
