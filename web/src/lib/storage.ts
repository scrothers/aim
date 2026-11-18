/**
 * Per-browser preferences in localStorage. Every access is guarded: storage
 * can be missing, full or blocked (private windows, previews), and the
 * console must render the same without it.
 */
export function readPref<T>(key: string, fallback: T, valid: (v: unknown) => v is T): T {
  try {
    const raw = localStorage.getItem(key)
    if (raw === null) return fallback
    const v: unknown = JSON.parse(raw)
    return valid(v) ? v : fallback
  } catch {
    return fallback
  }
}

export function writePref(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // Storage unavailable: the preference lasts for this page only.
  }
}

export const isBool = (v: unknown): v is boolean => typeof v === 'boolean'
export const isNumber = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v)
export const isStringArray = (v: unknown): v is string[] => Array.isArray(v) && v.every((x) => typeof x === 'string')
