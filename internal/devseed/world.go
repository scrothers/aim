package devseed

import (
	"fmt"
	"strings"

	"github.com/scrothers/aim/internal/address"
)

// Canonical role keys used by the scenario generators.
const (
	roleCoordinator = "coordinator"
	roleReviewer    = "reviewer"
	roleBuilder     = "builder"
	roleTriage      = "triage"
	roleDeployer    = "deployer"
	roleDocs        = "docs"
	roleQA          = "qa"
	roleSecurity    = "security"
	roleRelease     = "release"
	roleResearcher  = "researcher"
)

// persona is one simulated identity: who it is, who it reports to, and which
// MCP client and machine its sessions come from.
type persona struct {
	addr      address.Address
	role      string
	title     string
	desc      string
	reports   string
	client    string
	machine   string
	subagents []string
	retired   bool
	weight    int
}

func (p *persona) String() string { return p.addr.String() }

// org returns host/org.
func (p *persona) org() string { return p.addr.Host + "/" + p.addr.Org }

// scope returns host/org[/repo].
func (p *persona) scope() string {
	if p.addr.Repo == "" {
		return p.org()
	}
	return p.org() + "/" + p.addr.Repo
}

// sessionID is the stable session id of this persona's agent.
func (p *persona) sessionID() string {
	return "seed-" + strings.NewReplacer("@", "-", "/", "-", ".", "-").Replace(p.addr.String())
}

type spec struct {
	addr, role, title, desc, reports, client, machine string
	subagents                                         []string
	retired                                           bool
	weight                                            int
}

const (
	acme   = "github.com/acme"
	labs   = "github.com/labs"
	foo    = "gitlab.com/foo"
	opAddr = "operator"
)

func specs() []spec {
	return []spec{
		{
			addr: "coordinator@" + acme, role: roleCoordinator, title: "Org coordinator, acme", reports: opAddr,
			client: "claude-code", machine: "devbox", weight: 5, subagents: []string{"researcher", "scribe"},
			desc: "Coordinates every acme repo and is the escalation point for the team.\n\n- Sets weekly priorities and keeps the **release train** moving\n- Owns the daily summary posted to `github.com/acme`\n- Escalates budget, access and risk decisions to `operator`",
		},
		{
			addr: "release-manager@" + acme, role: roleRelease, title: "Release manager", reports: "coordinator@" + acme,
			client: "codex", machine: "devbox", weight: 2,
			desc: "Cuts releases across api, web and mobile.\n\n1. Collect merged PRs since the last tag\n2. Draft the changelog\n3. Tag, publish and announce to the org",
		},
		{
			addr: "reviewer@" + acme + "/api", role: roleReviewer, title: "Code reviewer", reports: "coordinator@" + acme,
			client: "claude-code", machine: "devbox", weight: 4, subagents: []string{"fixer", "nitpicker"},
			desc: "Reviews every PR to **acme/api**.\n\n- Correctness first, style second\n- Delegates mechanical fixes to the `fixer` subagent\n- Blocks merges that lack tests",
		},
		{
			addr: "builder@" + acme + "/api", role: roleBuilder, title: "Feature builder", reports: "coordinator@" + acme,
			client: "codex", machine: "pi-01", weight: 5,
			desc: "Implements features and fixes in **acme/api** (Go, PostgreSQL).\n\n- Works in small PRs\n- Pairs with `reviewer` before merging",
		},
		{
			addr: "triage@" + acme + "/api", role: roleTriage, title: "Issue triage", reports: "coordinator@" + acme,
			client: "gemini-cli", machine: "pi-02", weight: 3,
			desc: "Sorts incoming issues for **acme/api**: reproduces, labels, and routes them to the right builder.",
		},
		{
			addr: "reviewer@" + acme + "/web", role: roleReviewer, title: "Code reviewer", reports: "coordinator@" + acme,
			client: "opencode", machine: "devbox", weight: 3, subagents: []string{"fixer"},
			desc: "Reviews PRs to **acme/web** (React, TypeScript). Watches bundle size and accessibility regressions.",
		},
		{
			addr: "builder@" + acme + "/web", role: roleBuilder, title: "Feature builder", reports: "coordinator@" + acme,
			client: "claude-code", machine: "devbox", weight: 5, subagents: []string{"researcher"},
			desc: "Builds UI features in **acme/web**.\n\n- Component library owner\n- Spawns `researcher` for API and browser-support questions",
		},
		{
			addr: "qa@" + acme + "/web", role: roleQA, title: "QA engineer", reports: "coordinator@" + acme,
			client: "cline", machine: "pi-03", weight: 3,
			desc: "Runs the Playwright suite against every web release candidate and files regressions with traces attached.",
		},
		{
			addr: "deployer@" + acme + "/infra", role: roleDeployer, title: "Deployer", reports: "coordinator@" + acme,
			client: "claude-code", machine: "devbox", weight: 3, subagents: []string{"planner"},
			desc: "Applies Terraform and rolls out services to staging and production.\n\n> Production deploys need an approved window.",
		},
		{
			addr: "security@" + acme + "/infra", role: roleSecurity, title: "Security reviewer", reports: "coordinator@" + acme,
			client: "grok-cli", machine: "pi-04", weight: 2,
			desc: "Tracks advisories and reviews IAM, network and secrets changes across acme.",
		},
		{
			addr: "builder@" + acme + "/mobile", role: roleBuilder, title: "Mobile builder", reports: "coordinator@" + acme,
			client: "kilo-code", machine: "devbox", weight: 3,
			desc: "Builds the iOS and Android apps in **acme/mobile** (Kotlin, Swift).",
		},
		{
			addr: "qa@" + acme + "/mobile", role: roleQA, title: "Mobile QA", reports: "coordinator@" + acme,
			client: "deepseek-cli", machine: "pi-01", weight: 2,
			desc: "Device-farm test runs and release-candidate sign-off for mobile.",
		},
		{
			addr: "coordinator@" + labs, role: roleCoordinator, title: "Org coordinator, labs", reports: opAddr,
			client: "claude-code", machine: "devbox", weight: 4, subagents: []string{"researcher"},
			desc: "Runs the labs research org: radar and notebook.\n\n- Keeps experiments time-boxed\n- Brings only decisions to `operator`",
		},
		{
			addr: "researcher@" + labs + "/radar", role: roleResearcher, title: "Research engineer", reports: "coordinator@" + labs,
			client: "gemini-cli", machine: "pi-02", weight: 3,
			desc: "Prototypes anomaly-detection models for **labs/radar** and writes up the results.",
		},
		{
			addr: "builder@" + labs + "/radar", role: roleBuilder, title: "Feature builder", reports: "coordinator@" + labs,
			client: "codex", machine: "pi-03", weight: 3,
			desc: "Productionizes radar prototypes: ingestion, scoring service, dashboards.",
		},
		{
			addr: "docs@" + labs + "/notebook", role: roleDocs, title: "Docs writer", reports: "coordinator@" + labs,
			client: "opencode", machine: "pi-04", weight: 2,
			desc: "Keeps the **labs/notebook** tutorials and API reference accurate and runnable.",
		},
		{
			addr: "reviewer@" + labs + "/notebook", role: roleReviewer, title: "Code reviewer", reports: "coordinator@" + labs,
			client: "claude-code", machine: "devbox", weight: 2, subagents: []string{"fixer"},
			desc: "Reviews notebook kernel and renderer changes.",
		},
		{
			addr: "coordinator@" + foo, role: roleCoordinator, title: "Org coordinator, foo", reports: opAddr,
			client: "codex", machine: "pi-01", weight: 3,
			desc: "Coordinates the small **foo/bar** team on GitLab.",
		},
		{
			addr: "builder@" + foo + "/bar", role: roleBuilder, title: "Feature builder", reports: "coordinator@" + foo,
			client: "cline", machine: "pi-02", weight: 3,
			desc: "Implements features in **foo/bar**.",
		},
		{
			addr: "reviewer@" + foo + "/bar", role: roleReviewer, title: "Code reviewer", reports: "coordinator@" + foo,
			client: "mcp-inspector", machine: "devbox", weight: 1,
			desc: "Reviews merge requests for **foo/bar**. Mostly driven by hand from the MCP inspector.",
		},
		{
			addr: "scout@" + acme + "/api", role: roleResearcher, title: "Dependency scout (retired)", reports: "coordinator@" + acme,
			client: "gemini-cli", machine: "pi-03", weight: 3, retired: true,
			desc: "Surveyed upgrade paths for api dependencies. Retired after the module consolidation.",
		},
		{
			addr: "migrator@" + acme + "/infra", role: roleBuilder, title: "Migration runner (retired)", reports: "coordinator@" + acme,
			client: "claude-code", machine: "pi-04", weight: 3, retired: true,
			desc: "Ran the EKS migration. Retired once the cutover completed.",
		},
	}
}

// buildWorld parses specs into personas.
func buildWorld() ([]*persona, error) {
	ss := specs()
	out := make([]*persona, 0, len(ss))
	for _, s := range ss {
		a, err := address.Parse(s.addr)
		if err != nil {
			return nil, fmt.Errorf("devseed: world: %w", err)
		}
		out = append(out, &persona{
			addr: a, role: s.role, title: s.title, desc: s.desc, reports: s.reports,
			client: s.client, machine: s.machine, subagents: s.subagents, retired: s.retired, weight: s.weight,
		})
	}
	return out, nil
}

// flavor is the repo-specific vocabulary used to build realistic content.
type flavor struct {
	titles []string
	files  []string
	tests  []string
	lang   string
}

func flavors() map[string]flavor {
	return map[string]flavor{
		acme + "/api": {
			lang: "go",
			titles: []string{
				"Paginate /v2/orders with cursor tokens", "Fix N+1 query in invoice export", "Add idempotency keys to POST /payments",
				"Retry transient 503s from the ledger service", "Migrate sessions table to ULID primary keys", "Rate limit the public search endpoint",
			},
			files: []string{
				"internal/orders/list.go", "internal/payments/handler.go", "internal/ledger/client.go",
				"internal/billing/export.go", "internal/httpx/ratelimit.go", "migrations/0042_sessions_ulid.sql",
			},
			tests: []string{"TestListOrdersCursor", "TestPaymentIdempotency", "TestLedgerRetryBackoff", "TestInvoiceExportQueryCount", "TestRateLimitBurst"},
		},
		acme + "/web": {
			lang: "ts",
			titles: []string{
				"Virtualize the orders table", "Fix focus trap in the checkout dialog", "Add dark mode tokens to the design system",
				"Lazy-load the charts bundle", "Debounce the global search box", "Replace moment with date-fns",
			},
			files: []string{
				"src/components/OrdersTable.tsx", "src/components/CheckoutDialog.tsx", "src/theme/tokens.ts",
				"src/routes/dashboard.tsx", "src/hooks/useSearch.ts", "src/lib/dates.ts",
			},
			tests: []string{"OrdersTable > renders 10k rows without jank", "CheckoutDialog > traps focus", "useSearch > debounces input", "dates > formats relative times"},
		},
		acme + "/infra": {
			lang: "tf",
			titles: []string{
				"Move staging to a private subnet", "Rotate the RDS master credentials", "Add alarms for queue depth",
				"Upgrade EKS node group to 1.31", "Tighten the S3 bucket policy for exports",
			},
			files: []string{
				"terraform/staging/network.tf", "terraform/prod/rds.tf", "terraform/modules/queue/alarms.tf",
				"terraform/prod/eks.tf", "terraform/modules/exports/bucket.tf",
			},
			tests: []string{"tflint", "terraform validate", "checkov CKV_AWS_18", "conftest policy/iam"},
		},
		acme + "/mobile": {
			lang: "kt",
			titles: []string{
				"Fix crash when resuming a backgrounded checkout", "Support Android 15 predictive back", "Cache product images on disk",
				"Add biometric unlock", "Reduce cold-start time on low-end devices",
			},
			files: []string{
				"app/src/main/kotlin/checkout/CheckoutViewModel.kt", "app/src/main/kotlin/nav/BackHandler.kt",
				"ios/Sources/Cache/ImageCache.swift", "ios/Sources/Auth/Biometric.swift",
			},
			tests: []string{"CheckoutViewModelTest.resumesAfterProcessDeath", "BackHandlerTest.predictiveBack", "ImageCacheTests.evictsLRU"},
		},
		labs + "/radar": {
			lang: "py",
			titles: []string{
				"Add seasonal decomposition to the scorer", "Batch the feature store reads", "Switch the ingest worker to async",
				"Calibrate alert thresholds per tenant", "Backfill 90 days of embeddings",
			},
			files: []string{
				"radar/scoring/seasonal.py", "radar/features/store.py", "radar/ingest/worker.py", "radar/alerts/thresholds.py",
			},
			tests: []string{"tests/test_seasonal.py::test_weekly_cycle", "tests/test_store.py::test_batch_read", "tests/test_worker.py::test_backpressure"},
		},
		labs + "/notebook": {
			lang: "ts",
			titles: []string{
				"Render Mermaid diagrams in cells", "Fix kernel restart losing the output buffer", "Add keyboard shortcuts cheat sheet",
				"Stream cell output over WebSocket", "Document the kernel protocol",
			},
			files: []string{
				"packages/renderer/src/mermaid.ts", "packages/kernel/src/session.ts", "docs/reference/kernel-protocol.md", "packages/ui/src/shortcuts.tsx",
			},
			tests: []string{"session > preserves buffer across restart", "mermaid > renders flowchart", "shortcuts > lists all bindings"},
		},
		foo + "/bar": {
			lang: "py",
			titles: []string{
				"Add health endpoint", "Pin base image by digest", "Handle empty CSV uploads", "Cache the lookup table",
			},
			files: []string{"bar/api/health.py", "Dockerfile", "bar/ingest/csv.py", "bar/lookup.py"},
			tests: []string{"tests/test_health.py::test_ok", "tests/test_csv.py::test_empty_file", "tests/test_lookup.py::test_cache_ttl"},
		},
	}
}
