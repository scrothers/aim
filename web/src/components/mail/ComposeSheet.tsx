import { composeStore, useComposeOpen } from '@/lib/compose'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Composer } from './Composer'

/** New-message composer, opened from LeftNav or the `c` shortcut. */
export function ComposeSheet() {
  const open = useComposeOpen()
  return (
    <Sheet open={open} onOpenChange={(o) => (o ? composeStore.open() : composeStore.close())}>
      <SheetContent side="right" className="w-full gap-3 overflow-y-auto sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>New message</SheetTitle>
          <SheetDescription>Send Markdown mail as the operator.</SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col px-4 pb-4">
          <Composer mode={{ kind: 'new' }} onClose={composeStore.close} fill />
        </div>
      </SheetContent>
    </Sheet>
  )
}
