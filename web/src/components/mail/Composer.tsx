import { useRef, useState, type KeyboardEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Eye, Pencil, Send } from 'lucide-react'
import { ApiError } from '@/api/client'
import { renderQuery, rosterQuery, useSend, type MessageView } from '@/api/mail'
import { isValidAddress, OPERATOR } from '@/lib/address'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { RecipientInput } from './RecipientInput'

export type ComposerMode = { kind: 'new' } | { kind: 'reply'; original: MessageView }

const uniq = (xs: string[]) => [...new Set(xs)]

/** Reply-all prefill: to = original sender, cc = other recipients, operator excluded. */
export function replyDefaults(m: MessageView): { to: string[]; cc: string[]; subject: string } {
  const others = uniq([...(m.to ?? []), ...(m.cc ?? [])]).filter((a) => a !== OPERATOR)
  const to = m.from === OPERATOR ? (m.to ?? []).filter((a) => a !== OPERATOR) : [m.from]
  const cc = others.filter((a) => !to.includes(a))
  const subject = /^re:\s/i.test(m.subject) ? m.subject : `Re: ${m.subject}`
  return { to, cc, subject }
}

export function Composer({ mode, onClose, onSent }: { mode: ComposerMode; onClose: () => void; onSent?: () => void }) {
  const reply = mode.kind === 'reply' ? mode.original : undefined
  const defaults = reply ? replyDefaults(reply) : { to: [], cc: [], subject: '' }
  const [to, setTo] = useState<string[]>(defaults.to)
  const [cc, setCc] = useState<string[]>(defaults.cc)
  const [showCc, setShowCc] = useState(defaults.cc.length > 0)
  const [subject, setSubject] = useState(defaults.subject)
  const [body, setBody] = useState('')
  const [preview, setPreview] = useState(false)
  const [suggestions, setSuggestions] = useState<string[]>([])
  const [error, setError] = useState('')
  const toPending = useRef('')
  const ccPending = useRef('')
  const send = useSend()
  const roster = useQuery(rosterQuery())
  const options = (roster.data?.items ?? []).map((r) => r.address)
  const rendered = useQuery({ ...renderQuery(body), enabled: preview && body.trim() !== '' })

  const submit = () => {
    if (send.isPending) return
    const flush = (list: string[], pending: string) => {
      const t = pending.trim()
      return t && isValidAddress(t) && !list.includes(t) ? [...list, t] : list
    }
    const finalTo = flush(to, toPending.current)
    const finalCc = flush(cc, ccPending.current)
    if (finalTo.length + finalCc.length === 0) return setError('Add at least one recipient.')
    if (!subject.trim()) return setError('Subject is required.')
    if (!body.trim()) return setError('Write a message first.')
    setError('')
    setSuggestions([])
    send.mutate(
      {
        to: finalTo,
        cc: finalCc,
        subject: subject.trim(),
        body,
        ...(reply ? { in_reply_to: reply.id } : {}),
      },
      {
        onSuccess: () => {
          onSent?.()
          onClose()
        },
        onError: (e) => {
          const msg = e instanceof ApiError ? e.message : 'Could not send the message.'
          setError(msg)
          if (e instanceof ApiError) setSuggestions(e.suggestions)
          toast.error(msg)
        },
      },
    )
  }

  const applySuggestion = (s: string) => {
    const bad = [...to, ...cc].find((a) => error.includes(a))
    if (bad && to.includes(bad)) setTo(uniq(to.map((a) => (a === bad ? s : a))))
    else if (bad) setCc(uniq(cc.map((a) => (a === bad ? s : a))))
    else setTo(uniq([...to, s]))
    setSuggestions([])
    setError('')
  }

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      submit()
    } else if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
    }
  }

  return (
    <form
      aria-label={reply ? 'Reply' : 'New message'}
      onKeyDown={onKeyDown}
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
      className="flex flex-col gap-2.5 rounded-xl border bg-panel p-3"
    >
      <RecipientInput label="To" value={to} onChange={setTo} options={options} pendingRef={toPending} />
      {showCc ? (
        <RecipientInput label="Cc" value={cc} onChange={setCc} options={options} pendingRef={ccPending} />
      ) : (
        <button type="button" onClick={() => setShowCc(true)} className="self-start pl-10 text-xs text-accent-text hover:underline">
          Add Cc
        </button>
      )}
      {!reply && (
        <div className="flex items-center gap-2">
          <label htmlFor="composer-subject" className="w-8 shrink-0 text-xs font-semibold text-faint">
            Subj
          </label>
          <Input id="composer-subject" aria-label="Subject" value={subject} onChange={(e) => setSubject(e.target.value)} />
        </div>
      )}
      {preview ? (
        <div
          aria-label="Preview"
          className="prose-aim min-h-32 rounded-md border bg-panel2 p-3"
          // Sanitized server-side by /api/render.
          dangerouslySetInnerHTML={{ __html: rendered.data?.html ?? '' }}
        />
      ) : (
        <Textarea
          aria-label="Message body"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Write in Markdown…"
          rows={8}
          className="min-h-32 font-mono text-[13px]"
        />
      )}
      {error && (
        <div role="alert" className="text-[13px] text-destructive">
          <p className="m-0">{error}</p>
          {suggestions.length > 0 && (
            <p className="m-0 mt-1 flex flex-wrap items-center gap-1.5 text-text">
              Did you mean
              {suggestions.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => applySuggestion(s)}
                  className="rounded-full bg-accent-soft px-2 py-0.5 font-mono text-xs text-accent-text hover:underline"
                >
                  {s}
                </button>
              ))}
              ?
            </p>
          )}
        </div>
      )}
      <div className="flex items-center gap-2">
        <span className="text-xs text-faint">Sending as operator</span>
        <span className="flex-1" />
        <Button type="button" variant="ghost" size="sm" aria-pressed={preview} onClick={() => setPreview((p) => !p)}>
          {preview ? <Pencil aria-hidden="true" /> : <Eye aria-hidden="true" />}
          {preview ? 'Edit' : 'Preview'}
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={onClose}>
          Discard
        </Button>
        <Button type="submit" size="sm" disabled={send.isPending}>
          <Send aria-hidden="true" />
          Send <kbd className="font-sans text-xs opacity-80">⌘↵</kbd>
        </Button>
      </div>
    </form>
  )
}
