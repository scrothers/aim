import { useEffect, useState, type FormEvent } from 'react'
import { Check, X } from 'lucide-react'
import { useApprovals, useApprovalSettings, useDecideApproval, useSaveApprovalSettings, type Approval, type Decision } from '@/api/approvals'
import { describeInput } from '@/lib/approvalInput'
import { ago } from '@/lib/identity'
import { PageHeader } from '@/components/shared/PageHeader'
import { RuntimeBadge } from '@/components/shared/RuntimeBadge'
import { CountBadge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])
  return now
}

export function remaining(expiresAt: string, now: number): string | null {
  const s = Math.ceil((Date.parse(expiresAt) - now) / 1000)
  if (Number.isNaN(s) || s <= 0) return null
  return s >= 60 ? `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, '0')}s` : `${s}s`
}

export function ApprovalInput({ input }: { input: unknown }) {
  const v = describeInput(input)
  const pre = 'm-0 max-h-60 overflow-auto rounded-md bg-code p-2.5 font-mono text-xs break-words whitespace-pre-wrap'
  switch (v.kind) {
    case 'none':
      return <p className="m-0 text-[13px] text-faint">No input.</p>
    case 'command':
      return (
        <pre className={pre}>
          <code>{v.command}</code>
        </pre>
      )
    case 'file':
      return (
        <p className="m-0 text-[13px]">
          <code className="font-mono break-all">{v.path}</code>
          {v.note && <span className="text-muted-foreground"> · {v.note}</span>}
        </p>
      )
    case 'truncated':
      return (
        <div className="flex flex-col gap-1.5">
          <p className="m-0 text-xs text-faint">Input truncated ({v.bytes} bytes)</p>
          <pre className={pre}>
            <code>{v.preview}</code>
          </pre>
        </div>
      )
    case 'json':
      return (
        <pre className={pre}>
          <code>{v.text}</code>
        </pre>
      )
  }
}

function ApprovalCard({ a, now }: { a: Approval; now: number }) {
  const decide = useDecideApproval()
  const [reason, setReason] = useState('')
  const left = remaining(a.expires_at, now)
  const send = (decision: Decision) => decide.mutate({ id: a.id, decision, reason: reason.trim() })
  return (
    <li data-testid="approval" aria-label={`Approval ${a.tool}`} className="flex min-w-0 flex-col gap-3 rounded-xl border bg-panel p-4">
      <header className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
        <RuntimeBadge client={a.client} size={16} />
        {a.identity ? (
          <b className="font-mono text-[13px]">{a.identity}</b>
        ) : (
          <span className="text-[13px]">
            <b>unsigned session</b>
            {a.session_id && <span className="font-mono text-xs text-faint"> {a.session_id}</span>}
          </span>
        )}
        <span className="rounded bg-code px-1.5 font-mono text-[11px] leading-[18px]">{a.tool}</span>
        <span className="ml-auto text-xs text-faint">
          {ago(a.created_at, now)} ago ·{' '}
          {left ? <span>{left} left</span> : <span role="status">falling back to terminal</span>}
        </span>
      </header>
      <p className="m-0 truncate font-mono text-xs text-faint" title={a.project_dir}>
        {a.machine}
        {a.project_dir ? `:${a.project_dir}` : ''}
      </p>
      {a.summary && <p className="m-0 text-[13px]">{a.summary}</p>}
      <ApprovalInput input={a.input} />
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
        }}
      >
        <Input
          aria-label="Reason (optional)"
          placeholder="Reason (optional)"
          maxLength={500}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          className="min-w-0 flex-[1_1_200px]"
        />
        {/* One row at every width. Secondary then primary, as in the app's dialogs (Cancel, Confirm). */}
        <div role="group" aria-label="Decision" className="flex gap-2 max-md:w-full">
          <Button type="button" variant="outline" className="max-md:flex-1" disabled={decide.isPending} onClick={() => send('deny')}>
            <X aria-hidden="true" />
            Deny
          </Button>
          <Button type="button" className="max-md:flex-1" disabled={decide.isPending} onClick={() => send('allow')}>
            <Check aria-hidden="true" />
            Allow
          </Button>
        </div>
      </form>
    </li>
  )
}

export function ApprovalsPage() {
  const q = useApprovals()
  const settings = useApprovalSettings().data
  const save = useSaveApprovalSettings()
  const now = useNow()
  const items = q.data?.items ?? []
  return (
    <section aria-label="Approvals" className="box-border flex min-w-0 flex-1 flex-col gap-4 p-6 max-md:p-4">
      <PageHeader title="Approvals" badge={<CountBadge count={items.length} attention label="pending" />} />
      {q.isError ? (
        <p role="alert" className="m-0 text-destructive">
          Could not load approvals.
        </p>
      ) : !q.data ? (
        <p className="m-0 text-faint">Loading…</p>
      ) : items.length > 0 ? (
        <ul className="m-0 grid list-none grid-cols-[repeat(auto-fill,minmax(min(100%,460px),1fr))] gap-3 p-0">
          {items.map((a) => (
            <ApprovalCard key={a.id} a={a} now={now} />
          ))}
        </ul>
      ) : settings && !settings.enabled ? (
        <div className="flex flex-col items-start gap-3">
          <p className="m-0 text-muted-foreground">
            Remote approvals are off. Agents ask for permission in their own terminal. Turn them on to approve prompts from here.
          </p>
          <Button disabled={save.isPending} onClick={() => save.mutate({ enabled: true, timeout_seconds: settings.timeout_seconds })}>
            Turn on remote approvals
          </Button>
        </div>
      ) : (
        <p className="m-0 text-muted-foreground">Nothing pending. Agent permission prompts appear here while they wait for you.</p>
      )}
    </section>
  )
}
