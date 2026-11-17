// Package maintenance runs the daemon's housekeeping: pruning old archived
// mail under the configured retention and writing scheduled online backups.
// The same Runner backs the aim backup and aim maintenance commands and the
// console's "Back up now".
package maintenance
