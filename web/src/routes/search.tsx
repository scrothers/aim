import { createFileRoute } from '@tanstack/react-router'
import { useCallback } from 'react'
import { z } from 'zod'
import type { SearchPageParams } from '@/api/mail'
import { SearchPage } from '@/components/search/SearchPage'

const str = z.string().optional().catch(undefined)

export const searchPageSchema = z.object({
  q: str,
  scope: str,
  from: str,
  since: str,
  until: str,
  state: z.enum(['inbox', 'unread', 'archived', 'all']).optional().catch(undefined),
})

export const Route = createFileRoute('/search')({
  validateSearch: searchPageSchema,
  component: SearchRoute,
})

function SearchRoute() {
  const s = Route.useSearch()
  const navigate = Route.useNavigate()
  const params: SearchPageParams = { ...s, q: s.q ?? '' }
  const onSearch = useCallback((p: SearchPageParams) => void navigate({ search: p }), [navigate])
  return <SearchPage params={params} onSearch={onSearch} />
}
