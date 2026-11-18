import { useSyncExternalStore } from 'react'

/** Matches Tailwind's `md` breakpoint: below it the console is single-pane. */
export const MOBILE_QUERY = '(max-width: 767px)'
/** Matches Tailwind's `xl` breakpoint: from here the top bar and buddy rail have room. */
export const WIDE_QUERY = '(min-width: 1280px)'

function mq(query: string): MediaQueryList | undefined {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' ? window.matchMedia(query) : undefined
}

export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (cb) => {
      const m = mq(query)
      m?.addEventListener('change', cb)
      return () => m?.removeEventListener('change', cb)
    },
    () => mq(query)?.matches ?? false,
    () => false,
  )
}

export function useIsMobile(): boolean {
  return useMediaQuery(MOBILE_QUERY)
}

/** True at `xl` and up (and never on a phone, even when a test stubs every query to match). */
export function useIsWide(): boolean {
  const wide = useMediaQuery(WIDE_QUERY)
  const mobile = useIsMobile()
  return wide && !mobile
}
