import { useSyncExternalStore } from 'react'

export interface CreateDraft {
  title?: string
  scope?: string
  /** Message to link at creation (create from a thread). */
  messageId?: string
}

interface State {
  /** Task shown in the detail drawer. */
  selected?: string
  /** Create-task sheet: undefined while closed. */
  create?: CreateDraft
}

let state: State = {}
const listeners = new Set<() => void>()

function set(next: State) {
  state = next
  listeners.forEach((l) => l())
}

/** Shared UI state so the board, thread view, nav and palette can open the drawer and create sheet. */
export const taskStore = {
  get: () => state,
  select: (id: string | undefined) => set({ ...state, selected: id }),
  openCreate: (draft: CreateDraft = {}) => set({ ...state, create: draft }),
  closeCreate: () => set({ ...state, create: undefined }),
  reset: () => set({}),
}

export function useTaskStore(): State {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    () => state,
  )
}
