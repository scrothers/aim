import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Bookmark, MoreHorizontal, Search } from 'lucide-react'
import { filtersQuery, isSearchFilter, mailSearchFromParams, searchFromParams, useDeleteFilter, useRenameFilter, type SavedFilter } from '@/api/filters'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'

const rowCls =
  'flex h-8 max-md:min-h-11 min-w-0 flex-1 items-center gap-2.5 rounded-md px-2.5 text-muted-foreground hover:bg-hover data-[status=active]:bg-accent-soft data-[status=active]:font-semibold data-[status=active]:text-accent-text'

function FilterLink({ filter }: { filter: SavedFilter }) {
  const body = (
    <>
      {isSearchFilter(filter.params) ? <Search aria-hidden="true" className="size-4 shrink-0" /> : <Bookmark aria-hidden="true" className="size-4 shrink-0" />}
      <span className="min-w-0 flex-1 truncate">{filter.name}</span>
    </>
  )
  return isSearchFilter(filter.params) ? (
    <Link to="/search" search={searchFromParams(filter.params)} className={rowCls}>
      {body}
    </Link>
  ) : (
    <Link to="/mail" search={mailSearchFromParams(filter.params)} className={rowCls}>
      {body}
    </Link>
  )
}

function RenameDialog({ filter, onClose }: { filter?: SavedFilter; onClose: () => void }) {
  const rename = useRenameFilter()
  const [name, setName] = useState(filter?.name ?? '')
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (filter && name.trim()) rename.mutate({ id: filter.id, name: name.trim() }, { onSuccess: onClose })
  }
  return (
    <Dialog open={!!filter} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        {filter && (
          <form onSubmit={submit} className="flex flex-col gap-3">
            <DialogHeader>
              <DialogTitle>Rename view</DialogTitle>
              <DialogDescription>The view keeps its filters; only the name changes.</DialogDescription>
            </DialogHeader>
            <label className="flex flex-col gap-1.5 text-[13px] font-semibold">
              Name
              <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus required />
            </label>
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={onClose}>
                Cancel
              </Button>
              <Button type="submit" disabled={!name.trim() || rename.isPending}>
                Save
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

/** Saved filters, shared server-side, listed under the built-in views. */
export function SavedViews() {
  const items = useQuery(filtersQuery()).data?.items ?? []
  const del = useDeleteFilter()
  const [renaming, setRenaming] = useState<SavedFilter>()
  if (items.length === 0) return null
  return (
    <>
      {items.map((f) => (
        <div key={f.id} data-testid="saved-view" className="flex items-center">
          <FilterLink filter={f} />
          <DropdownMenu>
            <DropdownMenuTrigger
              aria-label={`Actions for ${f.name}`}
              className="flex size-7 shrink-0 items-center justify-center rounded-md text-faint hover:bg-hover max-md:size-11"
            >
              <MoreHorizontal aria-hidden="true" className="size-3.5" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => setRenaming(f)}>Rename</DropdownMenuItem>
              <DropdownMenuItem variant="destructive" onSelect={() => del.mutate(f.id)}>
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ))}
      <RenameDialog key={renaming?.id ?? 'none'} filter={renaming} onClose={() => setRenaming(undefined)} />
    </>
  )
}
