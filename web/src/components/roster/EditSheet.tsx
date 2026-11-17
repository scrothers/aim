import { useId, useState } from 'react'
import { ApiError } from '@/api/client'
import { useUpdateIdentity, type RosterEntry } from '@/api/roster'
import { OPERATOR } from '@/lib/address'
import { AddressChip } from '@/components/shared/AddressChip'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'

function EditForm({ entry, options, onClose }: { entry: RosterEntry; options: string[]; onClose: () => void }) {
  const [role, setRole] = useState(entry.role)
  const [description, setDescription] = useState(entry.description)
  const [reportsTo, setReportsTo] = useState(entry.reports_to ?? '')
  const [error, setError] = useState('')
  const update = useUpdateIdentity()
  const listId = useId()

  const save = () => {
    setError('')
    update.mutate(
      { address: entry.address, body: { role, description, reports_to: reportsTo.trim() } },
      {
        onSuccess: onClose,
        onError: (e) => setError(e instanceof ApiError ? e.message : 'Could not save changes.'),
      },
    )
  }

  return (
    <form
      className="flex flex-col gap-4 px-4 pb-4"
      onSubmit={(e) => {
        e.preventDefault()
        save()
      }}
    >
      <label className="flex flex-col gap-1.5 text-[13px] font-semibold">
        Role
        <Input value={role} onChange={(e) => setRole(e.target.value)} />
      </label>
      <label className="flex flex-col gap-1.5 text-[13px] font-semibold">
        Description
        <Textarea rows={6} value={description} onChange={(e) => setDescription(e.target.value)} />
      </label>
      <label className="flex flex-col gap-1.5 text-[13px] font-semibold">
        Reports to
        <Input
          list={listId}
          value={reportsTo}
          placeholder="operator"
          className="font-mono"
          onChange={(e) => setReportsTo(e.target.value)}
        />
        <datalist id={listId}>
          {options.map((o) => (
            <option key={o} value={o} />
          ))}
        </datalist>
        <span className="text-xs font-normal text-faint">Leave empty to clear. Escalations climb this chain to the operator.</span>
      </label>
      {error && (
        <p role="alert" className="m-0 text-[13px] text-destructive">
          {error}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={update.isPending}>
          {update.isPending ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </form>
  )
}

/** Side sheet for editing one identity's role, description and reports_to. */
export function EditSheet({ entry, roster, onClose }: { entry?: RosterEntry; roster: RosterEntry[]; onClose: () => void }) {
  const options = [OPERATOR, ...roster.map((r) => r.address).filter((a) => a !== entry?.address)]
  return (
    <Sheet open={!!entry} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right" className="w-full gap-3 overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>Edit identity</SheetTitle>
          <SheetDescription asChild>
            <div>{entry && <AddressChip address={entry.address} />}</div>
          </SheetDescription>
        </SheetHeader>
        {entry && <EditForm key={entry.address} entry={entry} options={options} onClose={onClose} />}
      </SheetContent>
    </Sheet>
  )
}
