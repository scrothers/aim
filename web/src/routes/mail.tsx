import { createFileRoute } from '@tanstack/react-router'
import { useCallback } from 'react'
import { z } from 'zod'
import { MailView } from '@/components/mail/MailView'

export const mailSearchSchema = z.object({
  view: z.enum(['operator', 'unread', 'today', 'sent', 'archived']).optional().catch(undefined),
  mailbox: z.string().optional().catch(undefined),
  state: z.enum(['inbox', 'unread', 'all', 'archived']).optional().catch(undefined),
  thread: z.string().optional().catch(undefined),
  muted: z.boolean().optional().catch(undefined),
})

export type MailSearch = z.infer<typeof mailSearchSchema>

export const Route = createFileRoute('/mail')({
  validateSearch: mailSearchSchema,
  component: MailPage,
})

function MailPage() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const onSearch = useCallback(
    (patch: Partial<MailSearch>) => void navigate({ search: (prev) => ({ ...prev, ...patch }) }),
    [navigate],
  )
  return <MailView search={search} onSearch={onSearch} />
}
