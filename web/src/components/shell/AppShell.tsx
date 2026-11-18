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

export function AppShell({ children }: { children: ReactNode }) {
  useGlobalShortcuts()
  const mobile = useIsMobile()
  return (
    <div className="flex min-h-screen flex-col bg-bg text-text">
      <TopBar />
      <div className="flex min-h-0 flex-1 flex-wrap">
        {!mobile && <LeftNav />}
        <main className="flex min-w-0 flex-[999_1_640px] flex-wrap max-md:flex-1">{children}</main>
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
