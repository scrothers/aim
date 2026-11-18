Who and where this connection is: session id, machine, project directory, git remote, git namespace (for example `github.com/acme/api`), MCP client, and the identity it is signed on as, or "nobody" with a hint to `agent_sign_on`.

Same content as the `agent_whoami` tool; attach it by hand, or call `agent_whoami` from a model. The namespace is the natural scope for `agent_roster`, `mail_list`, `mail_search` and `mail_wait` and for registering identities. Static: no subscribe.
