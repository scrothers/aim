import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

export const mailSearchSchema = z.object({
  view: z.enum(['operator', 'unread', 'today', 'sent', 'archived']).optional().catch(undefined),
  mailbox: z.string().optional().catch(undefined),
  state: z.enum(['unread', 'all', 'archived']).optional().catch(undefined),
  thread: z.string().optional().catch(undefined),
})

export type MailSearch = z.infer<typeof mailSearchSchema>

export const Route = createFileRoute('/mail')({
  validateSearch: mailSearchSchema,
  component: MailPage,
})

function MailPage() {
  const search = Route.useSearch()
  const title = search.mailbox ?? search.view ?? 'operator'
  return (
    <>
      <section
        aria-label="Message list"
        className="box-border flex min-w-0 flex-[1_1_340px] flex-col border-r bg-panel p-4 text-[13px] text-faint"
      >
        Message list for <span className="font-mono">{title}</span> arrives in the next step.
      </section>
      <article aria-label="Thread" className="box-border flex min-w-0 flex-[999_1_420px] flex-col bg-bg p-5 text-[13px] text-faint">
        Thread view arrives in the next step.
      </article>
    </>
  )
}
