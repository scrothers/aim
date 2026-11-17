import type { ReactNode } from 'react'
import { ComposeSheet } from '@/components/mail/ComposeSheet'
import { CommandPalette } from './CommandPalette'
import { OperatorAlerts } from './OperatorAlerts'
import { ShortcutsDialog, useGlobalShortcuts } from './Shortcuts'
import { BuddyRail } from './BuddyRail'
import { LeftNav } from './LeftNav'
import { TopBar } from './TopBar'

export function AppShell({ children }: { children: ReactNode }) {
  useGlobalShortcuts()
  return (
    <div className="flex min-h-screen flex-col bg-bg text-text">
      <TopBar />
      <div className="flex min-h-0 flex-1 flex-wrap">
        <LeftNav />
        <main className="flex min-w-0 flex-[999_1_640px] flex-wrap">{children}</main>
        <BuddyRail />
        <ComposeSheet />
        <CommandPalette />
        <ShortcutsDialog />
        <OperatorAlerts />
      </div>
    </div>
  )
}
