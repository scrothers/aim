import type { ReactNode } from 'react'
import { BuddyRail } from './BuddyRail'
import { LeftNav } from './LeftNav'
import { TopBar } from './TopBar'

export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col bg-bg text-text">
      <TopBar />
      <div className="flex min-h-0 flex-1 flex-wrap">
        <LeftNav />
        <main className="flex min-w-0 flex-[999_1_640px] flex-wrap">{children}</main>
        <BuddyRail />
      </div>
    </div>
  )
}
