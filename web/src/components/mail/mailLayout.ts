import { useLayoutEffect, useState, type RefObject } from 'react'
import { isNumber, readPref, writePref } from '@/lib/storage'

/**
 * Below this window width the list and the thread never share the screen:
 * one of them shows, full width, with Back (as on phones).
 */
export const NARROW_QUERY = '(max-width: 1099px)'
export const LIST_DEFAULT = 420
export const LIST_MIN = 300
/** The thread keeps at least this much room next to the list. */
export const THREAD_MIN = 380
/**
 * From 1100px they sit side by side, as long as the mail pane itself has
 * room for both (it shrinks with the nav and the buddy rail).
 */
export const SPLIT_MIN = LIST_MIN + THREAD_MIN
export const LIST_WIDTH_KEY = 'aim.mail.listWidth'

export type MailLayout = 'split' | 'single'

/** An unmeasured pane (0, e.g. before layout) counts as roomy. */
export function mailLayout(paneWidth: number, narrowViewport: boolean): MailLayout {
  if (narrowViewport) return 'single'
  return paneWidth === 0 || paneWidth >= SPLIT_MIN ? 'split' : 'single'
}

export function clampListWidth(w: number, paneWidth: number): number {
  const max = paneWidth > 0 ? Math.max(LIST_MIN, paneWidth - THREAD_MIN) : 640
  return Math.round(Math.min(Math.max(w, LIST_MIN), max))
}

/** Tracks an element's content-box width. */
export function useElementWidth(ref: RefObject<HTMLElement | null>): number {
  const [width, setWidth] = useState(0)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    setWidth(el.getBoundingClientRect().width)
    const ro = new ResizeObserver((entries) => {
      for (const e of entries) setWidth(e.contentRect.width)
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [ref])
  return width
}

/** The list width, remembered per browser. */
export function useListWidth() {
  const [width, setWidth] = useState(() => readPref(LIST_WIDTH_KEY, LIST_DEFAULT, isNumber))
  const save = (w: number) => {
    setWidth(w)
    writePref(LIST_WIDTH_KEY, w)
  }
  return [width, save] as const
}
