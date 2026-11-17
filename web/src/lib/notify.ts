import { useSyncExternalStore } from 'react'

const KEY = 'aim-notify'
const listeners = new Set<() => void>()

function read(): boolean {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}

let enabled = read()

export const notifySupported = () => typeof Notification !== 'undefined'

/** Preference only counts when the browser has actually granted permission. */
export function notificationsActive(): boolean {
  return enabled && notifySupported() && Notification.permission === 'granted'
}

function store(v: boolean) {
  enabled = v
  try {
    localStorage.setItem(KEY, v ? '1' : '0')
  } catch {
    /* storage unavailable */
  }
  listeners.forEach((l) => l())
}

/** Turns notifications on (requesting permission if needed) or off. Resolves to the resulting state. */
export async function setNotificationsEnabled(on: boolean): Promise<boolean> {
  if (!on || !notifySupported()) {
    store(false)
    return false
  }
  let perm = Notification.permission
  if (perm === 'default') perm = await Notification.requestPermission()
  const granted = perm === 'granted'
  store(granted)
  return granted
}

export function useNotificationsEnabled(): boolean {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    () => enabled,
  )
}

/** Test hook: re-reads the persisted preference. */
export function reloadNotifyPref() {
  enabled = read()
  listeners.forEach((l) => l())
}
