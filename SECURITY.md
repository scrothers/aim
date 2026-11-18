# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately. Do not open a public issue.

Use GitHub's private vulnerability reporting: open the repository's **Security** tab and choose **Report a vulnerability**
(or go to <https://github.com/scrothers/aim/security/advisories/new>).

Include the affected version or commit, what you did, and what you saw. This is a one-person project; expect an
acknowledgement within about a week and a fix or a status update as time allows.

## Supported versions

Only the latest `main` and the latest release receive security fixes.

## Threat model

The web console and HTTP API have no authentication. While remote approvals are on, anyone who can reach the console's
port can approve agent tool calls, including shell commands. AIM is meant for trusted networks; bind `AIM_HTTP` to
`127.0.0.1` or leave remote approvals off otherwise. See the security note under "Remote approvals" in
[Hooks and approvals](docs/hooks.md#remote-approvals).

Reports that depend on an attacker already having network access to an exposed console are known limitations,
not vulnerabilities, though suggestions to harden the defaults are welcome as ordinary issues.
