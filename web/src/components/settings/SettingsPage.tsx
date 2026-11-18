import { useState, type FormEvent, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Pencil, Plus, Send, Trash2 } from 'lucide-react'
import { approvalSettingsQuery, useSaveApprovalSettings } from '@/api/approvals'
import {
  settingsQuery,
  useBackupNow,
  usePrune,
  useSaveBackups,
  useSaveNotify,
  useSaveRetention,
  useTestWebhook,
  type PruneResult,
  type Settings,
  type TestResult,
  type WebhookInput,
  type WebhookView,
} from '@/api/settings'
import { ago } from '@/lib/identity'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

const when = (iso?: string) => (iso ? new Date(iso).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' }) : 'never')

function size(n?: number): string {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${u[i]}`
}

function Card({ title, hint, children, className }: { title: string; hint?: string; children: ReactNode; className?: string }) {
  return (
    <section aria-label={title} className={`box-border flex min-w-0 flex-col gap-3 rounded-xl border bg-panel p-4 ${className ?? ''}`}>
      <header className="flex flex-col gap-0.5">
        <h2 className="m-0 text-base font-semibold">{title}</h2>
        {hint && <p className="m-0 text-[13px] text-muted-foreground">{hint}</p>}
      </header>
      {children}
    </section>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5 text-[13px] font-semibold">
      {label}
      {children}
      {hint && <span className="text-xs font-normal text-faint">{hint}</span>}
    </label>
  )
}

function RetentionCard({ settings }: { settings: Settings }) {
  const save = useSaveRetention()
  const prune = usePrune()
  const [days, setDays] = useState(String(settings.retention.prune_archived_after_days))
  const [preview, setPreview] = useState<PruneResult>()
  const n = Number(days)
  const valid = Number.isInteger(n) && n >= 0

  const openPreview = () => prune.mutate(true, { onSuccess: setPreview })

  return (
    <Card title="Retention" hint="Delete messages older than N days once every recipient has archived them. Unarchived mail is never pruned.">
      <form
        className="flex flex-col gap-3"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          if (valid) save.mutate(n)
        }}
      >
        <Field label="Prune archived mail after (days)" hint="0 turns pruning off.">
          <Input type="number" min={0} step={1} value={days} onChange={(e) => setDays(e.target.value)} aria-invalid={!valid} />
        </Field>
        <div className="flex flex-wrap justify-end gap-2">
          <Button type="button" variant="outline" disabled={prune.isPending} onClick={openPreview}>
            Prune now
          </Button>
          <Button type="submit" disabled={!valid || save.isPending}>
            Save
          </Button>
        </div>
      </form>
      <Dialog open={!!preview} onOpenChange={(o) => !o && setPreview(undefined)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Prune archived mail?</DialogTitle>
            <DialogDescription>
              {!preview?.enabled
                ? 'Retention is off. Set a number of days above and save first.'
                : preview.messages === 0
                  ? `Nothing to prune: no fully archived messages are older than ${when(preview.cutoff)}.`
                  : `${preview.messages} ${preview.messages === 1 ? 'message' : 'messages'} older than ${when(preview.cutoff)} that every recipient archived will be deleted. This cannot be undone.`}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setPreview(undefined)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={!preview?.enabled || preview.messages === 0 || prune.isPending}
              onClick={() => prune.mutate(false, { onSuccess: () => setPreview(undefined) })}
            >
              Delete {preview?.messages ?? 0}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

function BackupsCard({ settings }: { settings: Settings }) {
  const save = useSaveBackups()
  const now = useBackupNow()
  const b = settings.backups
  const st = settings.backup_status
  const [hours, setHours] = useState(String(b.every_hours))
  const [keep, setKeep] = useState(String(b.keep))
  const [dir, setDir] = useState(b.dir)
  const h = Number(hours)
  const k = Number(keep)
  const valid = Number.isInteger(h) && h >= 0 && Number.isInteger(k) && k >= 1

  return (
    <Card title="Backups" hint="Consistent snapshots of the database while the daemon runs.">
      <form
        className="flex flex-col gap-3"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          if (valid) save.mutate({ every_hours: h, keep: k, dir })
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Field label="Every (hours)" hint="0 turns scheduled backups off.">
            <Input type="number" min={0} step={1} value={hours} onChange={(e) => setHours(e.target.value)} />
          </Field>
          <Field label="Keep" hint="Newest backups retained.">
            <Input type="number" min={1} step={1} value={keep} onChange={(e) => setKeep(e.target.value)} />
          </Field>
        </div>
        <Field label="Directory" hint={`Empty uses the default: ${b.effective_dir}`}>
          <Input value={dir} placeholder={b.effective_dir} className="font-mono" onChange={(e) => setDir(e.target.value)} />
        </Field>
        <dl className="m-0 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-[13px]" aria-label="Last backup">
          <dt className="text-faint">Last backup</dt>
          <dd className="m-0">
            {st.last_success_at ? `${when(st.last_success_at)} · ${size(st.last_size)}` : 'never'}
          </dd>
          {st.last_error && (
            <>
              <dt className="text-faint">Last error</dt>
              <dd role="alert" className="m-0 text-destructive">
                {st.last_error}
              </dd>
            </>
          )}
        </dl>
        <div className="flex flex-wrap justify-end gap-2">
          <Button type="button" variant="outline" disabled={now.isPending} onClick={() => now.mutate()}>
            {now.isPending ? 'Backing up…' : 'Back up now'}
          </Button>
          <Button type="submit" disabled={!valid || save.isPending}>
            Save
          </Button>
        </div>
      </form>
    </Card>
  )
}

interface Draft {
  id?: string
  name: string
  url: string
  masked?: string
  enabled: boolean
}

function WebhookDialog({
  draft,
  busy,
  error,
  onSave,
  onClose,
}: {
  draft?: Draft
  busy: boolean
  error?: string
  onSave: (d: Draft) => void
  onClose: () => void
}) {
  return (
    <Dialog open={!!draft} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        {draft && <WebhookForm key={draft.id ?? 'new'} draft={draft} busy={busy} error={error} onSave={onSave} onClose={onClose} />}
      </DialogContent>
    </Dialog>
  )
}

function WebhookForm({ draft, busy, error, onSave, onClose }: { draft: Draft; busy: boolean; error?: string; onSave: (d: Draft) => void; onClose: () => void }) {
  const [name, setName] = useState(draft.name)
  const [url, setUrl] = useState('')
  const creating = !draft.id
  return (
    <form
      className="flex flex-col gap-3"
      onSubmit={(e: FormEvent) => {
        e.preventDefault()
        onSave({ ...draft, name: name.trim(), url: url.trim() })
      }}
    >
      <DialogHeader>
        <DialogTitle>{creating ? 'Add webhook' : 'Edit webhook'}</DialogTitle>
        <DialogDescription>Discord and Slack incoming webhooks are supported.</DialogDescription>
      </DialogHeader>
      <Field label="Name">
        <Input value={name} onChange={(e) => setName(e.target.value)} required />
      </Field>
      <Field label="Webhook URL" hint={creating ? 'Stored on the daemon and never shown again.' : 'Write-only. Leave empty to keep the stored URL.'}>
        <Input
          type="url"
          value={url}
          required={creating}
          placeholder={draft.masked ?? 'https://discord.com/api/webhooks/…'}
          autoComplete="off"
          className="font-mono"
          onChange={(e) => setUrl(e.target.value)}
        />
      </Field>
      {error && (
        <p role="alert" className="m-0 text-[13px] text-destructive">
          {error}
        </p>
      )}
      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={busy || !name.trim() || (creating && !url.trim())}>
          {busy ? 'Saving…' : 'Save'}
        </Button>
      </DialogFooter>
    </form>
  )
}

function WebhookRow({
  hook,
  result,
  testing,
  onToggle,
  onEdit,
  onDelete,
  onTest,
}: {
  hook: WebhookView
  result?: TestResult
  testing: boolean
  onToggle: (on: boolean) => void
  onEdit: () => void
  onDelete: () => void
  onTest: () => void
}) {
  const failed = result ? !!result.error || result.status >= 400 : hook.last_error ? true : false
  const line = result
    ? result.error
      ? `Test failed: ${result.error}`
      : `Test sent: HTTP ${result.status}`
    : hook.last_sent_at
      ? hook.last_error
        ? `Last send ${ago(hook.last_sent_at)} ago failed: ${hook.last_error}`
        : `Last send ${ago(hook.last_sent_at)} ago: HTTP ${hook.last_status}`
      : 'Never sent'
  return (
    <li data-testid="webhook" className="flex flex-col gap-2 rounded-lg border bg-panel2 p-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className="min-w-0 flex-1 basis-48">
          <span className="flex items-center gap-2">
            <b className="truncate">{hook.name}</b>
            <span className="rounded bg-code px-1.5 text-[11px] leading-[18px] text-muted-foreground">{hook.kind}</span>
          </span>
          <span className="block truncate font-mono text-xs text-faint" title={hook.url}>
            {hook.url}
          </span>
        </span>
        <label className="flex items-center gap-2 text-[13px] text-muted-foreground">
          <input type="checkbox" checked={hook.enabled} onChange={(e) => onToggle(e.target.checked)} aria-label={`Enable ${hook.name}`} />
          Enabled
        </label>
        <div className="flex gap-1.5">
          <Button variant="outline" size="sm" disabled={testing} onClick={onTest}>
            <Send aria-hidden="true" />
            {testing ? 'Sending…' : 'Send test'}
          </Button>
          <Button variant="ghost" size="icon-sm" aria-label={`Edit ${hook.name}`} onClick={onEdit}>
            <Pencil aria-hidden="true" />
          </Button>
          <Button variant="ghost" size="icon-sm" aria-label={`Delete ${hook.name}`} onClick={onDelete}>
            <Trash2 aria-hidden="true" />
          </Button>
        </div>
      </div>
      <p role={failed && result ? 'alert' : undefined} className={`m-0 text-xs ${failed ? 'text-destructive' : 'text-faint'}`}>
        {line}
      </p>
    </li>
  )
}

function NotifyCard({ settings }: { settings: Settings }) {
  const cfg = settings.notify
  const save = useSaveNotify()
  const test = useTestWebhook()
  const [publicUrl, setPublicUrl] = useState(cfg.public_url ?? '')
  const [draft, setDraft] = useState<Draft>()
  const [confirm, setConfirm] = useState<WebhookView>()
  const [error, setError] = useState<string>()
  const [results, setResults] = useState<Record<string, TestResult>>({})
  const hooks = cfg.webhooks ?? []

  const inputs = (): WebhookInput[] => hooks.map((h) => ({ id: h.id, name: h.name, enabled: h.enabled }))
  const push = (webhooks: WebhookInput[], onSuccess?: () => void) =>
    save.mutate({ public_url: publicUrl.trim(), webhooks }, { onSuccess, onError: (e: unknown) => setError(e instanceof Error ? e.message : 'Save failed') })

  return (
    <Card
      title="Notifications"
      hint="New operator mail is posted to Discord or Slack as a card. Bursts are coalesced and muted threads stay quiet."
      className="col-span-full"
    >
      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          push(inputs())
        }}
      >
        <div className="min-w-0 flex-[1_1_320px]">
          <Field label="Public URL" hint={`Used for links in cards. Default: ${cfg.default_public_url ?? 'unknown'}`}>
            <Input type="url" value={publicUrl} placeholder={cfg.default_public_url} className="font-mono" onChange={(e) => setPublicUrl(e.target.value)} />
          </Field>
        </div>
        <Button type="submit" disabled={save.isPending}>
          Save
        </Button>
      </form>
      <div className="flex items-center gap-2">
        <h3 className="m-0 flex-1 text-[13px] font-semibold">Webhooks</h3>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            setError(undefined)
            setDraft({ name: '', url: '', enabled: true })
          }}
        >
          <Plus aria-hidden="true" />
          Add webhook
        </Button>
      </div>
      {hooks.length === 0 ? (
        <p className="m-0 text-[13px] text-faint">No webhooks configured.</p>
      ) : (
        <ul className="m-0 grid list-none grid-cols-[repeat(auto-fill,minmax(min(100%,420px),1fr))] gap-3 p-0">
          {hooks.map((h) => (
            <WebhookRow
              key={h.id}
              hook={h}
              result={results[h.id]}
              testing={test.isPending && test.variables === h.id}
              onToggle={(on) => push(inputs().map((i) => (i.id === h.id ? { ...i, enabled: on } : i)))}
              onEdit={() => {
                setError(undefined)
                setDraft({ id: h.id, name: h.name, url: '', masked: h.url, enabled: h.enabled })
              }}
              onDelete={() => setConfirm(h)}
              onTest={() => test.mutate(h.id, { onSuccess: (r) => setResults((s) => ({ ...s, [h.id]: r })) })}
            />
          ))}
        </ul>
      )}
      <WebhookDialog
        draft={draft}
        busy={save.isPending}
        error={error}
        onClose={() => setDraft(undefined)}
        onSave={(d) => {
          const next: WebhookInput = { ...(d.id ? { id: d.id } : {}), name: d.name, enabled: d.enabled, ...(d.url ? { url: d.url } : {}) }
          const list = d.id ? inputs().map((i) => (i.id === d.id ? next : i)) : [...inputs(), next]
          push(list, () => setDraft(undefined))
        }}
      />
      <Dialog open={!!confirm} onOpenChange={(o) => !o && setConfirm(undefined)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete webhook?</DialogTitle>
            <DialogDescription>{confirm?.name} will stop receiving notifications. The stored URL is discarded.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setConfirm(undefined)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                const id = confirm?.id
                setConfirm(undefined)
                push(inputs().filter((i) => i.id !== id))
              }}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

function ApprovalsCard({ enabled, timeoutSeconds }: { enabled: boolean; timeoutSeconds: number }) {
  const save = useSaveApprovalSettings()
  const [on, setOn] = useState(enabled)
  const [minutes, setMinutes] = useState(String(timeoutSeconds / 60))
  const secs = Math.round(Number(minutes) * 60)
  const valid = minutes.trim() !== '' && Number.isFinite(secs) && secs >= 10 && secs <= 3600
  return (
    <Card
      title="Remote approvals"
      hint="Off: agents prompt for permission in their terminal. On: prompts wait here for your decision up to the timeout, then fall back to the terminal."
    >
      <form
        className="flex flex-col gap-3"
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          if (valid) save.mutate({ enabled: on, timeout_seconds: secs })
        }}
      >
        <label className="flex items-center gap-2 text-[13px] font-semibold">
          <input type="checkbox" checked={on} onChange={(e) => setOn(e.target.checked)} />
          Enable remote approvals
        </label>
        <Field label="Timeout (minutes)" hint="Between 10 seconds and 60 minutes. Default 5.">
          <Input type="number" min={1 / 6} max={60} step="any" value={minutes} onChange={(e) => setMinutes(e.target.value)} aria-invalid={!valid} />
        </Field>
        <div className="flex justify-end">
          <Button type="submit" disabled={!valid || save.isPending}>
            Save
          </Button>
        </div>
      </form>
    </Card>
  )
}

export function SettingsPage() {
  const approvals = useQuery(approvalSettingsQuery()).data
  const q = useQuery(settingsQuery())
  const s = q.data
  return (
    <section aria-label="Settings" className="box-border flex min-w-0 flex-1 flex-col gap-4 p-6 max-md:p-4">
      <h1 className="m-0 text-[22px] leading-[30px] font-semibold">Settings</h1>
      {q.isError ? (
        <p role="alert" className="m-0 text-destructive">
          Could not load settings.
        </p>
      ) : !s ? (
        <p className="m-0 text-faint">Loading…</p>
      ) : (
        <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,380px),1fr))] items-start gap-4">
          {/* Remount when the saved values change so the forms reset to them. */}
          <RetentionCard key={`r${s.retention.prune_archived_after_days}`} settings={s} />
          <BackupsCard key={`b${s.backups.every_hours}/${s.backups.keep}/${s.backups.dir}`} settings={s} />
          {approvals && typeof approvals.enabled === 'boolean' && (
            <ApprovalsCard key={`a${approvals.enabled}/${approvals.timeout_seconds}`} enabled={approvals.enabled} timeoutSeconds={approvals.timeout_seconds || 300} />
          )}
          <NotifyCard key={`n${s.notify.public_url ?? ''}`} settings={s} />
        </div>
      )}
    </section>
  )
}
