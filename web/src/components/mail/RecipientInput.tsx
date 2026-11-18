import { useMemo, useState } from 'react'
import { Command as CommandPrimitive } from 'cmdk'
import { X } from 'lucide-react'
import { isValidAddress } from '@/lib/address'
import { cn } from '@/lib/utils'
import { AddressChip } from '@/components/shared/AddressChip'
import { CommandItem, CommandList } from '@/components/ui/command'
import { KIND_LABEL, rankCandidates, useRecipientCandidates } from './recipientCandidates'

export interface RecipientInputProps {
  label: string
  value: string[]
  onChange: (next: string[]) => void
  /** Exposes the pending (uncommitted) text so the composer can flush it on send. */
  pendingRef?: { current: string }
}

/**
 * Chip input: commits on Enter, comma, space, Tab or blur; validates with the
 * address grammar. While type-ahead suggestions are open, arrows move, and
 * Enter or Tab accepts the highlighted one.
 */
export function RecipientInput({ label, value, onChange, pendingRef }: RecipientInputProps) {
  const [text, setText] = useState('')
  const [error, setError] = useState('')
  const [focused, setFocused] = useState(false)
  const [dismissed, setDismissed] = useState(false)
  const [active, setActive] = useState('')
  const candidates = useRecipientCandidates()

  const setPending = (t: string) => {
    setText(t)
    if (pendingRef) pendingRef.current = t
  }

  const suggestions = useMemo(() => rankCandidates(candidates, text, value), [candidates, text, value])
  const open = focused && !dismissed && suggestions.length > 0
  // cmdk reports the highlight; fall back to the first row when it is stale.
  const current = suggestions.some((s) => s.address === active) ? active : (suggestions[0]?.address ?? '')

  const commit = (raw: string): boolean => {
    const addr = raw.trim().replace(/,$/, '')
    if (!addr) return true
    if (!isValidAddress(addr)) {
      setError(`Invalid address: ${addr}`)
      return false
    }
    setError('')
    if (!value.includes(addr)) onChange([...value, addr])
    setPending('')
    return true
  }

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-start gap-2">
        <span className="flex h-9 w-14 shrink-0 items-center text-xs font-semibold text-faint">{label}</span>
        <CommandPrimitive
          shouldFilter={false}
          loop
          label={`${label} recipients`}
          value={current}
          onValueChange={setActive}
          className="relative flex min-w-0 flex-1 flex-wrap items-center gap-1.5 rounded-md border bg-panel px-1.5 py-1 focus-within:border-ring focus-within:ring-2 focus-within:ring-ring"
        >
          {value.map((addr) => (
            <span key={addr} className="inline-flex items-center gap-1 rounded-full bg-accent-soft py-0.5 pr-1 pl-2.5 text-[13px]">
              <AddressChip address={addr} />
              <button
                type="button"
                aria-label={`Remove ${addr}`}
                onClick={() => onChange(value.filter((v) => v !== addr))}
                className="rounded-full p-0.5 text-faint hover:bg-hover hover:text-text"
              >
                <X aria-hidden="true" className="size-3" />
              </button>
            </span>
          ))}
          <CommandPrimitive.Input
            aria-label={`${label} recipients`}
            aria-invalid={error ? true : undefined}
            value={text}
            placeholder={value.length === 0 ? 'name@host/org/repo' : ''}
            onValueChange={(t) => {
              setError('')
              setDismissed(false)
              setPending(t)
            }}
            onFocus={() => setFocused(true)}
            onBlur={() => {
              setFocused(false)
              if (text.trim()) commit(text)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) return // send shortcut bubbles to the composer
              if (open && (e.key === 'Enter' || e.key === 'Tab') && current) {
                e.preventDefault() // also stops cmdk from handling Enter itself
                commit(current)
              } else if (open && e.key === 'Escape') {
                e.preventDefault()
                e.stopPropagation() // closes the list, not the composer
                setDismissed(true)
              } else if (e.key === 'Enter' || e.key === ',' || e.key === ' ' || (e.key === 'Tab' && text.trim())) {
                if (text.trim()) {
                  e.preventDefault()
                  commit(text)
                } else if (e.key === 'Enter') e.preventDefault()
              } else if (e.key === 'Backspace' && !text && value.length) {
                onChange(value.slice(0, -1))
              }
            }}
            className="min-w-40 flex-1 bg-transparent px-1 py-0.5 font-mono text-[13px] outline-none"
          />
          <CommandList
            label={`${label} suggestions`}
            hidden={!open}
            className="absolute top-full left-0 z-20 mt-1 w-full rounded-md border bg-popover py-1 shadow-md"
          >
            {open &&
              suggestions.map((s) => (
                <CommandItem
                  key={s.address}
                  value={s.address}
                  // Keep focus in the input so blur does not commit the typed text first.
                  onMouseDown={(e) => e.preventDefault()}
                  onSelect={() => commit(s.address)}
                  className="px-3 py-1.5 text-[13px] data-[selected=true]:bg-hover data-[selected=true]:text-foreground"
                >
                  {s.kind === 'identity' || s.kind === 'subagent' ? (
                    <span
                      role="img"
                      aria-label={s.online ? 'Online' : 'Offline'}
                      className={cn('size-2 shrink-0 rounded-full', s.online ? 'bg-online' : 'bg-faint/50')}
                    />
                  ) : (
                    <span aria-hidden="true" className="size-2 shrink-0" />
                  )}
                  <AddressChip address={s.address} className="min-w-0 truncate" />
                  <span className="ml-auto shrink-0 text-xs text-faint">
                    {s.kind === 'identity' && s.role ? s.role : KIND_LABEL[s.kind]}
                  </span>
                </CommandItem>
              ))}
          </CommandList>
        </CommandPrimitive>
      </div>
      {error && (
        <p role="alert" className="m-0 pl-10 text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  )
}
