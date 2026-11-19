// Package service renders and manages the per-user OS service that runs the
// AIM daemon: a systemd user unit on Linux and a launchd LaunchAgent on macOS.
//
// A [Manager] is pure: it renders the unit or plist and returns the commands
// to run as argv lists. The caller owns file writes and process execution, so
// tests never touch the host service manager.
package service
