import { useState } from 'react'
import { ChevronDown, ChevronRight, Monitor } from 'lucide-react'
import type { components } from '@/api/schema'
import { clock, parseAddress, runtimeFor } from '@/lib/identity'
import { cn } from '@/lib/utils'
import { PresenceDot } from '@/components/shared/PresenceDot'
import { RobotAvatar } from '@/components/shared/RobotAvatar'
import { RuntimeBadge } from '@/components/shared/RuntimeBadge'

type PresenceBody = components['schemas']['PresenceBody']
type Buddy = components['schemas']['Buddy']
type Group = components['schemas']['BuddyGroup']

/** `github.com/acme/api` -> `acme/api` */
export function groupLabel(name: string): string {
  const i = name.indexOf('/')
  return i >= 0 ? name.slice(i + 1) : name
}

export function onlineTotal(groups: ReadonlyArray<Group>): number {
  return groups.filter((g) => !g.offline).reduce((n, g) => n + g.online, 0)
}

function BuddyRow({ buddy }: { buddy: Buddy }) {
  const rt = runtimeFor(buddy.client)
  return (
    <li
      className={cn('flex flex-col gap-1 rounded-lg py-1 pr-1.5 pb-1.5 pl-5 hover:bg-hover', !buddy.online && 'opacity-55')}
      data-testid="buddy"
      data-online={buddy.online}
    >
      <span className="flex min-w-0 items-center gap-2">
        <PresenceDot online={buddy.online} />
        <RobotAvatar address={buddy.address} size={24} />
        <span className="min-w-0 truncate font-semibold">{buddy.name}</span>
        {buddy.online && <RuntimeBadge client={buddy.client} className="ml-auto" />}
      </span>
      {buddy.online && (
        <span className="ml-4 flex min-w-0 items-center gap-1 text-[11px] leading-[14px] whitespace-nowrap text-faint">
          <Monitor aria-hidden="true" className="size-3 shrink-0" />
          <span className="min-w-0 truncate font-mono" title={buddy.machine || 'unknown'}>
            {buddy.machine || 'unknown'}
          </span>
          <span className="shrink-0">· {rt.label}</span>
        </span>
      )}
      {buddy.online && buddy.away && (
        <span className="ml-4 self-start rounded-[12px_12px_12px_4px] bg-accent-soft px-2.5 py-1 text-[13px] leading-[18px] text-accent-text">
          {buddy.away}
        </span>
      )}
      {(buddy.subagents ?? []).length > 0 && (
        <span className="ml-4 flex flex-wrap gap-1">
          {(buddy.subagents ?? []).map((s) => (
            <span key={s} className="rounded bg-code px-1.5 font-mono text-[11px] leading-[18px] text-muted-foreground">
              ↳ {s}
            </span>
          ))}
        </span>
      )}
    </li>
  )
}

export function BuddyList({ data }: { data: PresenceBody }) {
  // Offline starts collapsed; online groups start open. Toggles flip the default.
  const [flipped, setFlipped] = useState<ReadonlySet<string>>(new Set())
  const toggle = (name: string) =>
    setFlipped((s) => {
      const n = new Set(s)
      if (n.has(name)) n.delete(name)
      else n.add(name)
      return n
    })

  const groups = data.groups ?? []
  const online = groups.filter((g) => !g.offline)
  const offline = groups.filter((g) => g.offline)
  const feed = (data.events ?? []).filter((e) => e.kind === 'signed_on' || e.kind === 'signed_off').slice(0, 5)

  const renderGroup = (g: Group) => {
    const open = g.offline ? flipped.has(g.name) : !flipped.has(g.name)
    const Chev = open ? ChevronDown : ChevronRight
    const id = `buddy-group-${g.name}`
    return (
      <section key={g.name} className="flex flex-col gap-2" aria-label={g.name}>
        <button
          type="button"
          aria-expanded={open}
          aria-controls={id}
          onClick={() => toggle(g.name)}
          className={cn(
            'flex items-center gap-1.5 px-1 text-left text-[13px] font-semibold',
            g.offline ? 'text-muted-foreground' : 'text-text',
          )}
          title={g.name}
        >
          <Chev aria-hidden="true" className="size-3.5 text-faint" />
          <span className={g.offline ? undefined : 'font-mono text-xs'}>{g.offline ? g.name : groupLabel(g.name)}</span>
          <span className="font-normal text-faint">({g.offline ? g.total || (g.buddies ?? []).length : `${g.online}/${g.total}`})</span>
        </button>
        {open && (
          <ul id={id} className="m-0 flex list-none flex-col gap-2 p-0">
            {(g.buddies ?? []).map((b) => (
              <BuddyRow key={`${b.address}|${b.machine ?? ''}|${b.signed_on_at ?? ''}`} buddy={b} />
            ))}
          </ul>
        )}
      </section>
    )
  }

  return (
    <>
      <div className="flex items-center gap-2 px-1">
        <h2 className="m-0 text-[15px] font-semibold">Buddy list</h2>
        <span className="ml-auto rounded-full bg-online-soft px-2 py-px text-xs font-semibold text-online-text">
          {onlineTotal(groups)} online
        </span>
      </div>
      {online.map(renderGroup)}
      {offline.map(renderGroup)}
      <div className="mt-auto flex flex-col gap-1 border-t pt-2.5 pr-1 pl-1 text-xs text-muted-foreground">
        {feed.map((e, i) => {
          const on = e.kind === 'signed_on'
          return (
            <span key={`${e.at}-${i}`} className="flex gap-1.5" data-testid="feed-event">
              <span aria-hidden="true" className={cn('font-bold', on ? 'text-online' : 'text-faint')}>
                {on ? '→' : '←'}
              </span>
              <span>
                <b>{parseAddress(e.identity ?? '').identity ?? e.identity}</b> {on ? 'signed on' : 'signed off'} · {clock(e.at)}
              </span>
            </span>
          )
        })}
        <span className="mt-1 text-faint">Refreshes every 5 seconds</span>
      </div>
    </>
  )
}
