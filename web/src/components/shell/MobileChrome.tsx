import { useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { buddiesFlag, navFlag } from '@/lib/flag'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { BuddyPanel } from './BuddyRail'
import { LeftNav } from './LeftNav'

/** Phone layout: the left nav and the buddy list live in sheets opened from the top bar. */
export function MobileChrome() {
  const navOpen = navFlag.use()
  const buddiesOpen = buddiesFlag.use()
  // Navigating (a link in the drawer) closes it.
  const href = useRouterState({ select: (s) => s.location.href })
  const [prev, setPrev] = useState(href)
  if (prev !== href) {
    setPrev(href)
    navFlag.set(false)
    buddiesFlag.set(false)
  }
  return (
    <>
      <Sheet open={navOpen} onOpenChange={navFlag.set}>
        <SheetContent side="left" className="w-[88%] gap-0 overflow-y-auto" showCloseButton>
          <SheetHeader className="sr-only">
            <SheetTitle>Navigation</SheetTitle>
            <SheetDescription>Views, mailboxes and settings</SheetDescription>
          </SheetHeader>
          <LeftNav drawer onNavigate={() => navFlag.set(false)} />
        </SheetContent>
      </Sheet>
      <Sheet open={buddiesOpen} onOpenChange={buddiesFlag.set}>
        <SheetContent side="right" className="w-[88%] gap-4 overflow-y-auto px-3 py-4" showCloseButton>
          <SheetHeader className="sr-only">
            <SheetTitle>Buddy list</SheetTitle>
            <SheetDescription>Who is online right now</SheetDescription>
          </SheetHeader>
          <BuddyPanel />
        </SheetContent>
      </Sheet>
    </>
  )
}
