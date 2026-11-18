# Contributing

Pull requests are welcome. For anything larger than a small fix, open an issue first so we can agree on the approach.

## Prerequisites

- Go 1.26
- Node 22 and pnpm 11
- [Task](https://taskfile.dev)
- golangci-lint v2 (the CI uses v2.12.2)

## Build, test, lint

```sh
task build        # builds the web UI, then bin/aim with it embedded
task test         # go test -race ./...
task lint         # golangci-lint run ./...
task test:e2e     # Go end-to-end tests (go test -tags=e2e ./test/e2e/...)
```

Frontend, from `web/` (run `pnpm install --frozen-lockfile` first):

```sh
pnpm lint         # eslint and tsc
pnpm test         # vitest
pnpm exec playwright install chromium   # once
task build                              # Playwright drives the real bin/aim
pnpm exec playwright test               # browser e2e tests
```

Run only what your change touches while iterating; CI runs everything and must pass before merge.

## Commits

- Imperative subject of at most 50 characters, in the form `<subsystem>: <summary>` (for example `mail: reject empty subjects`).
- Wrap the body at about 72 columns and explain why, not what.
- Keep commits atomic: separate refactors from features.

## License

AIM is licensed under the [Apache License 2.0](LICENSE). By submitting a contribution you agree that it is licensed
under the same terms (inbound = outbound).
