Who and where this connection is: session id, machine, project directory, git remote, git namespace (for example `github.com/acme/api`), MCP client, and the identity it is signed on as, or "nobody" with a hint to `sign_on`.

Same content as the `whoami` tool; attach it by hand, or call `whoami` from a model. The namespace is the natural scope for `roster`, `list`, `search` and `wait` and for registering identities. Static: no subscribe.
