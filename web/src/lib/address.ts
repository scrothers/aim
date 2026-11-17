/** Mirrors the daemon grammar `[subagent@][identity@]host/org[/repo]`, plus the reserved `operator`. */
const SEG = '[A-Za-z0-9][A-Za-z0-9._-]*'
const ADDRESS_RE = new RegExp(`^(?:operator|(?:${SEG}@){0,2}${SEG}/${SEG}(?:/${SEG})?)$`)

export function isValidAddress(addr: string): boolean {
  return ADDRESS_RE.test(addr)
}

export const OPERATOR = 'operator'
