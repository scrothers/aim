import { useEffect } from 'react'
import { composeStore } from '@/lib/compose'
import { helpFlag, isTyping, paletteFlag } from '@/lib/flag'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'

const SHORTCUTS: Array<[string, string]> = [
  ['⌘K / Ctrl+K', 'Open the command palette'],
  ['?', 'Show this help'],
  ['c', 'Compose a new message'],
  ['j / k', 'Next / previous message'],
  ['Enter', 'Open the selected thread'],
  ['r', 'Reply'],
  ['e', 'Archive (Unarchive in the archive view)'],
  ['u', 'Mark unread'],
  ['Esc', 'Close dialogs and the composer'],
]

/** Global keys: the palette toggles anywhere, `?` only when not typing. */
export function useGlobalShortcuts() {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        paletteFlag.toggle()
        return
      }
      if (e.metaKey || e.ctrlKey || e.altKey || isTyping(e.target)) return
      if (e.key === '?' && !composeStore.isOpen() && !paletteFlag.get()) {
        e.preventDefault()
        helpFlag.toggle()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
}

export function ShortcutsDialog() {
  const open = helpFlag.use()
  return (
    <Dialog open={open} onOpenChange={helpFlag.set}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>Shortcuts are ignored while you are typing in a field.</DialogDescription>
        </DialogHeader>
        <dl className="m-0 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-[13px]">
          {SHORTCUTS.map(([k, d]) => (
            <div key={k} className="contents">
              <dt>
                <kbd className="rounded border px-1.5 py-px font-mono text-xs">{k}</kbd>
              </dt>
              <dd className="m-0 text-muted-foreground">{d}</dd>
            </div>
          ))}
        </dl>
      </DialogContent>
    </Dialog>
  )
}
