import { usePresence } from '@/api/queries'
import { BuddyList } from './BuddyList'

/** Buddy list content, shared by the desktop rail and the mobile sheet. */
export function BuddyPanel() {
  const { data, isError } = usePresence()
  return data ? (
    <BuddyList data={data} />
  ) : (
    <p className="m-0 px-1 text-[13px] text-faint">{isError ? 'Buddy list unavailable.' : 'Loading…'}</p>
  )
}

export function BuddyRail() {
  return (
    <aside aria-label="Buddy list" className="box-border flex w-[264px] shrink-0 flex-col gap-4 overflow-y-auto border-l bg-panel px-3 py-4">
      <BuddyPanel />
    </aside>
  )
}
