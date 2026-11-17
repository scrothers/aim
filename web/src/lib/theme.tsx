import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

export type ThemePref = 'system' | 'light' | 'dark'
const KEY = 'aim-theme'

export function readPref(): ThemePref {
  try {
    const v = localStorage.getItem(KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* storage unavailable */
  }
  return 'system'
}

function writePref(p: ThemePref) {
  try {
    localStorage.setItem(KEY, p)
  } catch {
    /* storage unavailable */
  }
}

function systemDark(): boolean {
  return typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches
}

export function resolveDark(pref: ThemePref): boolean {
  return pref === 'dark' || (pref === 'system' && systemDark())
}

interface Ctx {
  pref: ThemePref
  dark: boolean
  setPref: (p: ThemePref) => void
  toggle: () => void
}

const ThemeContext = createContext<Ctx | null>(null)

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [pref, setPrefState] = useState<ThemePref>(readPref)
  const [sysDark, setSysDark] = useState(systemDark)

  useEffect(() => {
    if (typeof matchMedia !== 'function') return
    const mq = matchMedia('(prefers-color-scheme: dark)')
    const on = () => setSysDark(mq.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])

  const dark = pref === 'dark' || (pref === 'system' && sysDark)

  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
  }, [dark])

  const setPref = useCallback((p: ThemePref) => {
    writePref(p)
    setPrefState(p)
  }, [])
  const toggle = useCallback(() => setPref(dark ? 'light' : 'dark'), [dark, setPref])

  const value = useMemo(() => ({ pref, dark, setPref, toggle }), [pref, dark, setPref, toggle])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): Ctx {
  const c = useContext(ThemeContext)
  if (!c) throw new Error('useTheme outside ThemeProvider')
  return c
}
