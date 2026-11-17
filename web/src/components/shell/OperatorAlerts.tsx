import { useEffect, useRef } from 'react'
import { api, unwrap } from '@/api/client'
import { useViews } from '@/api/queries'
import { notificationsActive } from '@/lib/notify'

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
  const unread = useViews().data?.operator_unread
  const prev = useRef<number | undefined>(undefined)

  useEffect(() => {
    document.title = unread ? `(${unread}) AIM` : 'AIM'
  }, [unread])

  useEffect(() => {
    if (unread === undefined) return
    const before = prev.current
    prev.current = unread
    if (before !== undefined && unread > before && notificationsActive()) void announce().catch(() => undefined)
  }, [unread])

  return null
}
