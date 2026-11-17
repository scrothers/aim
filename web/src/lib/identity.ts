export interface ParsedAddress {
  subagent?: string
  identity?: string
  path: string
}

/** Splits `[subagent@][identity@]host/org[/repo]`; `operator` has no path. */
export function parseAddress(addr: string): ParsedAddress {
  const parts = addr.split('@')
  if (parts.length === 1) {
    return addr.includes('/') ? { path: addr } : { identity: addr, path: '' }
  }
  const path = parts[parts.length - 1] ?? ''
  if (parts.length === 3) return { subagent: parts[0], identity: parts[1], path }
  return { identity: parts[0], path }
}

/** Deterministic hue (0-359) from a name, stable across sessions. */
export function hueFor(name: string): number {
  let h = 2166136261
  for (let i = 0; i < name.length; i++) {
    h ^= name.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return (h >>> 0) % 360
}

export function initialsFor(name: string): string {
  const words = name.split(/[^a-z0-9]+/i).filter(Boolean)
  if (words.length >= 2) return ((words[0]![0] ?? '') + (words[1]![0] ?? '')).toUpperCase()
  return name.replace(/[^a-z0-9]/gi, '').slice(0, 2).toUpperCase() || '?'
}

export interface Runtime {
  /** Display name. */
  label: string
  /** Vendored icon file slug, or undefined for the generic fallback badge. */
  icon?: string
}

const RUNTIME_RULES: Array<[RegExp, Runtime]> = [
  [/^claude/i, { label: 'Claude Code', icon: 'claude' }],
  [/^codex/i, { label: 'Codex', icon: 'codex' }],
  [/^gemini/i, { label: 'Gemini CLI', icon: 'gemini' }],
  [/^opencode/i, { label: 'OpenCode', icon: 'opencode' }],
  [/^grok/i, { label: 'Grok', icon: 'grok' }],
  [/^cline/i, { label: 'Cline', icon: 'cline' }],
  [/^kilo/i, { label: 'Kilo Code', icon: 'kilo-code' }],
  [/^deepseek/i, { label: 'DeepSeek', icon: 'deepseek' }],
]

/** Maps an MCP clientInfo name to a runtime; unknown names keep the raw name. */
export function runtimeFor(client: string | undefined): Runtime {
  const name = (client ?? '').trim()
  if (!name) return { label: 'Unknown' }
  for (const [re, rt] of RUNTIME_RULES) if (re.test(name)) return rt
  return { label: name }
}

const icons = import.meta.glob('../assets/runtimes/*.svg', { eager: true, query: '?url', import: 'default' }) as Record<
  string,
  string
>

export function runtimeIconUrl(slug: string): string | undefined {
  return icons[`../assets/runtimes/${slug}.svg`]
}

/** Compact relative time: "now", "4s", "2m", "3h", "5d". */
export function ago(iso: string | undefined, now = Date.now()): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''
  const s = Math.max(0, Math.round((now - t) / 1000))
  if (s < 5) return 'now'
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 86400) return `${Math.floor(s / 3600)}h`
  return `${Math.floor(s / 86400)}d`
}

export function clock(iso: string | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false })
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${units[i]}`
}
