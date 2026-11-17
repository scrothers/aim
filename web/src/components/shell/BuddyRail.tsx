import { usePresence } from '@/api/queries'
import { BuddyList } from './BuddyList'

export function BuddyRail() {
  const { data, isError } = usePresence()
  return (
    <aside
      aria-label="Buddy list"
      className="box-border flex flex-[1_1_264px] flex-col gap-4 border-l bg-panel px-3 py-4"
    >
      {data ? (
        <BuddyList data={data} />
      ) : (
        <p className="m-0 px-1 text-[13px] text-faint">{isError ? 'Buddy list unavailable.' : 'Loading…'}</p>
      )}
    </aside>
  )
}
