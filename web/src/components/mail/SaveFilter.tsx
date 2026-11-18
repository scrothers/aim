import { useState, type FormEvent } from 'react'
import { Bookmark } from 'lucide-react'
import { useCreateFilter, type FilterParams } from '@/api/filters'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

/** "Save view" button: names the current params and stores them as a shared filter. */
export function SaveFilterButton({
  params,
  label = 'Save view',
  suggestion = '',
  size = 'sm',
}: {
  params: FilterParams
  label?: string
  suggestion?: string
  /** `default` in a page header, `sm` in the denser mail list header. */
  size?: 'sm' | 'default'
}) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const create = useCreateFilter()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const n = name.trim()
    if (n) create.mutate({ name: n, params }, { onSuccess: () => setOpen(false) })
  }
  return (
    <>
      <Button
        variant="outline"
        size={size}
        className="max-md:h-11"
        onClick={() => {
          setName(suggestion)
          setOpen(true)
        }}
      >
        <Bookmark aria-hidden="true" />
        {label}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <form onSubmit={submit} className="flex flex-col gap-3">
            <DialogHeader>
              <DialogTitle>{label}</DialogTitle>
              <DialogDescription>Saved views are shared with every console and appear in the left nav.</DialogDescription>
            </DialogHeader>
            <label className="flex flex-col gap-1.5 text-[13px] font-semibold">
              Name
              <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus required />
            </label>
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={!name.trim() || create.isPending}>
                Save
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
