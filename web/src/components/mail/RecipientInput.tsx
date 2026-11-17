import { useId, useMemo, useState } from 'react'
import { X } from 'lucide-react'
import { isValidAddress } from '@/lib/address'
import { cn } from '@/lib/utils'
import { AddressChip } from '@/components/shared/AddressChip'

export interface RecipientInputProps {
  label: string
  value: string[]
  onChange: (next: string[]) => void
  /** Known addresses offered as autocomplete. */
  options?: string[]
  /** Exposes the pending (uncommitted) text so the composer can flush it on send. */
  pendingRef?: { current: string }
}

/** Chip input: commits on Enter, comma, space, Tab or blur; validates with the address grammar. */
export function RecipientInput({ label, value, onChange, options = [], pendingRef }: RecipientInputProps) {
  const [text, setText] = useState('')
  const [error, setError] = useState('')
  const [focused, setFocused] = useState(false)
  const listId = useId()

  const setPending = (t: string) => {
    setText(t)
    if (pendingRef) pendingRef.current = t
  }

  const suggestions = useMemo(() => {
    const q = text.trim().toLowerCase()
    if (!q) return []
    return options.filter((o) => !value.includes(o) && o.toLowerCase().includes(q)).slice(0, 6)
  }, [text, options, value])

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
        <span className="w-8 shrink-0 pt-1.5 text-xs font-semibold text-faint">{label}</span>
        <div className="relative flex min-w-0 flex-1 flex-wrap items-center gap-1.5 rounded-md border bg-panel px-1.5 py-1 focus-within:ring-2 focus-within:ring-ring/40">
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
          <input
            aria-label={`${label} recipients`}
            aria-invalid={error ? true : undefined}
            aria-autocomplete="list"
            aria-controls={suggestions.length ? listId : undefined}
            value={text}
            placeholder={value.length === 0 ? 'name@host/org/repo' : ''}
            onChange={(e) => {
              setError('')
              setPending(e.target.value)
            }}
            onFocus={() => setFocused(true)}
            onBlur={() => {
              setFocused(false)
              if (text.trim()) commit(text)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) return // send shortcut bubbles to the composer
              if (e.key === 'Enter' || e.key === ',' || e.key === ' ' || (e.key === 'Tab' && text.trim())) {
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
          {focused && suggestions.length > 0 && (
            <ul
              id={listId}
              role="listbox"
              aria-label={`${label} suggestions`}
              className="absolute top-full left-0 z-20 mt-1 w-full overflow-hidden rounded-md border bg-popover py-1 shadow-md"
            >
              {suggestions.map((s) => (
                <li key={s} role="option" aria-selected={false}>
                  <button
                    type="button"
                    tabIndex={-1}
                    className={cn('flex w-full items-center px-3 py-1.5 text-left text-[13px] hover:bg-hover')}
                    onMouseDown={(e) => {
                      e.preventDefault()
                      commit(s)
                    }}
                  >
                    <AddressChip address={s} />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
      {error && (
        <p role="alert" className="m-0 pl-10 text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  )
}
