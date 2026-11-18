import type { ReactNode } from 'react'
import { ComposeSheet } from '@/components/mail/ComposeSheet'
import { CreateTaskSheet } from '@/components/tasks/CreateTaskSheet'
import { TaskDrawer } from '@/components/tasks/TaskDrawer'
import { useIsMobile } from '@/lib/viewport'
import { CommandPalette } from './CommandPalette'
import { OperatorAlerts } from './OperatorAlerts'
import { ShortcutsDialog, useGlobalShortcuts } from './Shortcuts'
import { BuddyRail } from './BuddyRail'
import { LeftNav } from './LeftNav'
import { MobileChrome } from './MobileChrome'
import { TopBar } from './TopBar'

/**
 * Viewport-high shell: the top bar and side columns stay put, and each
 * column scrolls on its own (the page inside <main>, the mailbox tree
 * inside the nav).
 */
export function AppShell({ children }: { children: ReactNode }) {
  useGlobalShortcuts()
  const mobile = useIsMobile()
  return (
    <div className="flex h-dvh flex-col overflow-hidden bg-bg text-text">
      <TopBar />
      <div className="flex min-h-0 flex-1">
        {!mobile && <LeftNav />}
        <main className="flex min-w-0 flex-1 flex-col overflow-y-auto">{children}</main>
        {!mobile && <BuddyRail />}
        {mobile && <MobileChrome />}
        <ComposeSheet />
        <TaskDrawer />
        <CreateTaskSheet />
        <CommandPalette />
        <ShortcutsDialog />
        <OperatorAlerts />
      </div>
    </div>
  )
}
