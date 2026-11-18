import { useId, useMemo, useState } from 'react'
import { cn } from '@/lib/utils'
import { AddressChip } from '@/components/shared/AddressChip'
import { Input } from '@/components/ui/input'
import { rankCandidates, useRecipientCandidates, type Candidate } from '@/components/mail/recipientCandidates'

export interface AssigneeInputProps {
  value: string
  onChange: (v: string) => void
  /** Enter with nothing highlighted. */
  onSubmit?: () => void
  label?: string
  placeholder?: string
  id?: string
  className?: string
}

/** Identities the operator may assign a task to (live identities and `operator`; no orgs, repos or subagents). */
export function useAssigneeCandidates(): Candidate[] {
  const all = useRecipientCandidates()
  return useMemo(() => all.filter((c) => c.kind === 'identity' || c.kind === 'operator'), [all])
}

/** Single-value type-ahead over identities. Free text is kept so the server can answer with suggestions. */
export function AssigneeInput({ value, onChange, onSubmit, label = 'Assignee', placeholder = 'identity@host/org', id, className }: AssigneeInputProps) {
  const candidates = useAssigneeCandidates()
  const [focused, setFocused] = useState(false)
  const [active, setActive] = useState(0)
  const listId = useId()
  const suggestions = useMemo(() => {
    const q = value.trim()
    if (!q) return candidates.slice(0, 8)
    const ranked = rankCandidates(candidates, q)
    // An exact match needs no list.
    return ranked.length === 1 && ranked[0]!.address === q ? [] : ranked
  }, [candidates, value])
  const open = focused && suggestions.length > 0
  const cur = Math.min(active, Math.max(suggestions.length - 1, 0))

  const pick = (c: Candidate) => {
    onChange(c.address)
    setFocused(false)
  }

  return (
    <div className={cn('relative min-w-0', className)}>
      <Input
        id={id}
        role="combobox"
        aria-label={label}
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={open ? `${listId}-${cur}` : undefined}
        autoComplete="off"
        spellCheck={false}
        value={value}
        placeholder={placeholder}
        onChange={(e) => {
          onChange(e.target.value)
          setActive(0)
          setFocused(true)
        }}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onKeyDown={(e) => {
          if (open && e.key === 'ArrowDown') {
            e.preventDefault()
            setActive((cur + 1) % suggestions.length)
          } else if (open && e.key === 'ArrowUp') {
            e.preventDefault()
            setActive((cur - 1 + suggestions.length) % suggestions.length)
          } else if (open && (e.key === 'Enter' || e.key === 'Tab') && suggestions[cur] && value.trim() && suggestions[cur]!.address !== value.trim()) {
            e.preventDefault()
            pick(suggestions[cur]!)
          } else if (open && e.key === 'Escape') {
            e.preventDefault()
            e.stopPropagation()
            setFocused(false)
          } else if (e.key === 'Enter') {
            e.preventDefault()
            onSubmit?.()
          }
        }}
        className="font-mono text-[13px]"
      />
      <ul
        id={listId}
        role="listbox"
        aria-label={`${label} suggestions`}
        hidden={!open}
        className="absolute top-full left-0 z-20 mt-1 m-0 max-h-60 w-full list-none overflow-auto rounded-md border bg-popover p-0 py-1 shadow-md"
      >
        {open &&
          suggestions.map((s, i) => (
            <li
              key={s.address}
              id={`${listId}-${i}`}
              role="option"
              aria-selected={i === cur}
              // Keep focus in the input so blur does not close the list before the click lands.
              onMouseDown={(e) => {
                e.preventDefault()
                pick(s)
              }}
              onMouseEnter={() => setActive(i)}
              className={cn('flex cursor-pointer items-center gap-2 px-3 py-1.5 text-[13px] max-md:min-h-11', i === cur && 'bg-hover')}
            >
              <span
                role="img"
                aria-label={s.online ? 'Online' : 'Offline'}
                className={cn('size-2 shrink-0 rounded-full', s.online ? 'bg-online' : 'bg-faint/50')}
              />
              <AddressChip address={s.address} className="min-w-0 truncate" />
              {s.role && <span className="ml-auto shrink-0 text-xs text-faint">{s.role}</span>}
            </li>
          ))}
      </ul>
    </div>
  )
}
