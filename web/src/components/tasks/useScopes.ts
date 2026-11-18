import { useMemo } from 'react'
import { useMailboxAddresses } from '@/api/queries'

/** Org and repo addresses a task can be scoped to. */
export function useScopes(): string[] {
  const all = useMailboxAddresses()
  return useMemo(() => all.filter((a) => !a.includes('@')), [all])
}
