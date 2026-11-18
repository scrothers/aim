import { useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { approvalsQuery } from '@/api/approvals'
import { api, unwrap } from '@/api/client'
import { viewsQuery } from '@/api/queries'
import { notificationsActive, useNotificationsEnabled } from '@/lib/notify'

async function announce(): Promise<void> {
  const res = unwrap(await api.GET('/api/messages', { params: { query: { address: 'operator', state: 'unread', limit: 1 } } }))
  const newest = res.items?.[0]
  if (!newest) return
  new Notification(newest.from, { body: newest.subject, tag: newest.id })
}

/**
 * Mirrors the operator unread count into the tab title and, when the user has
 * opted in, raises a browser notification when the count rises between polls.
 * Renders nothing.
 */
export function OperatorAlerts() {
  // Hidden tabs only keep polling when the user opted in to notifications.
  const background = useNotificationsEnabled() && notificationsActive()
  const unread = useQuery(viewsQuery(background)).data?.operator_unread
  const prev = useRef<number | undefined>(undefined)
  const approvals = useQuery(approvalsQuery(background)).data?.items
  const seen = useRef<Set<string> | undefined>(undefined)
  const pending = approvals?.length ?? 0

  useEffect(() => {
    const badge = [unread ? String(unread) : '', pending ? `${pending}!` : ''].filter(Boolean).join(' ')
    document.title = badge ? `(${badge}) AIM` : 'AIM'
  }, [unread, pending])

  useEffect(() => {
    if (!approvals) return
    const before = seen.current
    seen.current = new Set(approvals.map((a) => a.id))
    if (!before || !notificationsActive()) return
    for (const a of approvals.filter((x) => !before.has(x.id)).slice(0, 3)) {
      try {
        new Notification('Approval needed', { body: `${a.identity || a.client || 'agent'}: ${a.summary || a.tool}`, tag: a.id })
      } catch {
        /* notifications unavailable */
      }
    }
  }, [approvals])

  useEffect(() => {
    if (unread === undefined) return
    const before = prev.current
    prev.current = unread
    if (before !== undefined && unread > before && notificationsActive()) void announce().catch(() => undefined)
  }, [unread])

  return null
}
