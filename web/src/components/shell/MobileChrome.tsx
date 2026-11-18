import { useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { PenLine } from 'lucide-react'
import { composeStore } from '@/lib/compose'
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
    <>
      <ComposeFab />
      <Sheet open={navOpen} onOpenChange={navFlag.set}>
      <SheetContent side="left" className="w-[88%] gap-0" showCloseButton>
        <SheetHeader className="sr-only">
          <SheetTitle>Navigation</SheetTitle>
          <SheetDescription>Views, mailboxes and settings</SheetDescription>
        </SheetHeader>
        <LeftNav drawer onNavigate={() => navFlag.set(false)} />
      </SheetContent>
      </Sheet>
    </>
  )
}

/** Floating Compose on the mail list (an open thread has its own Reply). */
function ComposeFab() {
  const show = useRouterState({
    select: (s) => s.location.pathname === '/mail' && !(s.location.search as { thread?: unknown }).thread,
  })
  if (!show) return null
  return (
    <button
      type="button"
      aria-label="Compose"
      onClick={composeStore.open}
      className="fixed right-[max(1rem,env(safe-area-inset-right))] bottom-[max(1rem,env(safe-area-inset-bottom))] z-40 flex size-14 items-center justify-center rounded-full bg-brand text-on-accent shadow-lg"
    >
      <PenLine aria-hidden="true" className="size-5" />
    </button>
  )
}
