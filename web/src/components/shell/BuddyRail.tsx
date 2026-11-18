import { useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { usePresence } from '@/api/queries'
import { buddiesFlag, createFlag } from '@/lib/flag'
import { isBool, readPref, writePref } from '@/lib/storage'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { BuddyList } from './BuddyList'

/** Buddy list content, shared by the desktop rail and the sheet. */
export function BuddyPanel() {
  const { data, isError } = usePresence()
  return data ? (
    <BuddyList data={data} />
  ) : (
    <p className="m-0 px-1 text-[13px] text-faint">{isError ? 'Buddy list unavailable.' : 'Loading…'}</p>
  )
}

export const BUDDY_RAIL_KEY = 'aim.buddyRail.open'
/** Wide pages where the rail starts collapsed. */
export const RAIL_AUTO_COLLAPSED = ['/tasks', '/org-chart']

/** The operator's rail choice, remembered per browser (default open). */
export const railPref = createFlag(readPref(BUDDY_RAIL_KEY, true, isBool))
/** The rail on auto-collapsed pages: closed until opened, for this page load. */
export const railOnWidePages = createFlag(false)

/** Whether the inline rail (xl and up) is open on the current page, and its toggle. */
export function useBuddyRail() {
  const auto = useRouterState({ select: (s) => RAIL_AUTO_COLLAPSED.includes(s.location.pathname) })
  const pref = railPref.use()
  const widePages = railOnWidePages.use()
  const open = auto ? widePages : pref
  const toggle = () => {
    if (auto) {
      railOnWidePages.set(!open)
      return
    }
    railPref.set(!open)
    writePref(BUDDY_RAIL_KEY, !open)
  }
  return { open, toggle }
}

export function BuddyRail() {
  return (
    <aside aria-label="Buddy list" className="box-border flex w-[264px] shrink-0 flex-col gap-4 overflow-y-auto border-l bg-panel px-3 py-4">
      <BuddyPanel />
    </aside>
  )
}

/** Below xl the buddy list opens over the page instead of taking width. */
export function BuddySheet() {
  const open = buddiesFlag.use()
  // Navigating (a link in the sheet) closes it.
  const href = useRouterState({ select: (s) => s.location.href })
  const [prev, setPrev] = useState(href)
  if (prev !== href) {
    setPrev(href)
    buddiesFlag.set(false)
  }
  return (
    <Sheet open={open} onOpenChange={buddiesFlag.set}>
      <SheetContent side="right" className="w-[88%] gap-4 overflow-y-auto px-3 pb-4 sm:max-w-sm" showCloseButton>
        <SheetHeader className="sr-only">
          <SheetTitle>Buddy list</SheetTitle>
          <SheetDescription>Who is online right now</SheetDescription>
        </SheetHeader>
        <BuddyPanel />
      </SheetContent>
    </Sheet>
  )
}
