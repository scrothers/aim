import { useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { navFlag } from '@/lib/flag'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { LeftNav } from './LeftNav'

/** Phone layout: the left nav lives in a drawer opened from the top bar. */
export function MobileChrome() {
  const navOpen = navFlag.use()
  // Navigating (a link in the drawer) closes it.
  const href = useRouterState({ select: (s) => s.location.href })
  const [prev, setPrev] = useState(href)
  if (prev !== href) {
    setPrev(href)
    navFlag.set(false)
  }
  return (
    <Sheet open={navOpen} onOpenChange={navFlag.set}>
      <SheetContent side="left" className="w-[88%] gap-0" showCloseButton>
        <SheetHeader className="sr-only">
          <SheetTitle>Navigation</SheetTitle>
          <SheetDescription>Views, mailboxes and settings</SheetDescription>
        </SheetHeader>
        <LeftNav drawer onNavigate={() => navFlag.set(false)} />
      </SheetContent>
    </Sheet>
  )
}
