import { ShieldCheck } from 'lucide-react'
import { useApprovalSettings, useSaveApprovalSettings } from '@/api/approvals'
import { timeoutLabel } from '@/lib/approvals'
import { DropdownMenuCheckboxItem } from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

/** Remote-approvals setting plus its toggle; the pending count lives on the nav link. */
export function useRemoteApprovals() {
  const settings = useApprovalSettings().data
  const save = useSaveApprovalSettings()
  const enabled = settings?.enabled ?? false
  const timeout = settings?.timeout_seconds ?? 300
  return {
    enabled,
    ready: !!settings && !save.isPending,
    hint: enabled
      ? `On: agent prompts wait here up to ${timeoutLabel(timeout)}, then fall back to the terminal.`
      : 'Off: agents prompt in their terminal. Turn on to approve their prompts here.',
    toggle: () => save.mutate({ enabled: !enabled, timeout_seconds: timeout }),
  }
}

/** Header switch for remote approvals (wide screens). */
export function ApprovalsSwitch() {
  const { enabled, ready, hint, toggle } = useRemoteApprovals()
  return (
    <button
      type="button"
      role="switch"
      aria-checked={enabled}
      aria-label="Remote approvals"
      title={hint}
      disabled={!ready}
      onClick={toggle}
      className="flex h-8 shrink-0 items-center gap-2 rounded-lg border px-2 text-[13px] whitespace-nowrap text-muted-foreground hover:bg-hover disabled:opacity-50"
    >
      <ShieldCheck aria-hidden="true" className="size-4" />
      Remote approvals
      {/* On: the logo's colours, a yellow track with a near-black knob. */}
      <span aria-hidden="true" className={cn('relative h-4 w-7 rounded-full transition-colors', enabled ? 'bg-brand ring-1 ring-brand-strong ring-inset' : 'bg-border')}>
        <span className={cn('absolute top-0.5 size-3 rounded-full transition-all', enabled ? 'left-3.5 bg-on-accent' : 'left-0.5 bg-white')} />
      </span>
    </button>
  )
}

/** The same toggle as a More-menu item (narrow screens). */
export function ApprovalsMenuItem() {
  const { enabled, ready, hint, toggle } = useRemoteApprovals()
  return (
    <DropdownMenuCheckboxItem checked={enabled} disabled={!ready} title={hint} onCheckedChange={toggle}>
      <ShieldCheck aria-hidden="true" />
      Remote approvals
    </DropdownMenuCheckboxItem>
  )
}
