import { useSyncExternalStore } from 'react'

/** Matches Tailwind's `md` breakpoint: below it the console is single-pane. */
export const MOBILE_QUERY = '(max-width: 767px)'

function mq(): MediaQueryList | undefined {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' ? window.matchMedia(MOBILE_QUERY) : undefined
}

export function useIsMobile(): boolean {
  return useSyncExternalStore(
    (cb) => {
      const m = mq()
      m?.addEventListener('change', cb)
      return () => m?.removeEventListener('change', cb)
    },
    () => mq()?.matches ?? false,
    () => false,
  )
}
