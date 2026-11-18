package limits

// clientFloorBytes is the smallest cap any supported harness puts on server
// instructions and tool descriptions: Claude Code's 4,096 characters (since
// v2.1.296; Codex, Gemini CLI, Grok Build and OpenCode set no lower cap, as
// researched 2026-11-18).
const clientFloorBytes = 4096

// MaxDescriptionBytes caps the server instructions and each tool
// description, in UTF-8 bytes: the client floor less 10%. The headroom
// covers harnesses that count bytes rather than characters, truncation
// markers, and a future drop in the floor, so no client silently cuts our
// text.
const MaxDescriptionBytes = clientFloorBytes * 9 / 10

// MaxCatalogDescriptionBytes caps each resource, resource template, prompt
// and prompt argument description, in UTF-8 bytes. No harness caps these;
// this keeps them explicit without sprawling in pickers and slash-command
// menus.
const MaxCatalogDescriptionBytes = 1000
