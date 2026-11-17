import { useSyncExternalStore } from 'react'

/** Tiny shared boolean store for cross-component UI state (palette, help dialog). */
export function createFlag(initial = false) {
  let value = initial
  const listeners = new Set<() => void>()
  const set = (v: boolean) => {
    if (value === v) return
    value = v
    listeners.forEach((l) => l())
  }
  return {
    get: () => value,
    set,
    toggle: () => set(!value),
    use: () =>
      useSyncExternalStore(
        (cb) => {
          listeners.add(cb)
          return () => listeners.delete(cb)
        },
        () => value,
      ),
  }
}

export const paletteFlag = createFlag()
export const helpFlag = createFlag()

/** True when the event target is somewhere the user is typing. */
export function isTyping(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false
  return t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName)
}

/** Mobile: the left-nav drawer and the buddy-list sheet. */
export const navFlag = createFlag()
export const buddiesFlag = createFlag()
