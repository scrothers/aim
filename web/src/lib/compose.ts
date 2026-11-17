import { useSyncExternalStore } from 'react'

/** Tiny store so LeftNav, shortcuts and the sheet share the "new message" open state. */
let open = false
const listeners = new Set<() => void>()

function set(v: boolean) {
  if (open === v) return
  open = v
  listeners.forEach((l) => l())
}

export const composeStore = {
  open: () => set(true),
  close: () => set(false),
  isOpen: () => open,
}

export function useComposeOpen(): boolean {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    () => open,
  )
}
