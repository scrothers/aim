import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { RosterGrid } from '@/components/roster/RosterGrid'

export const Route = createFileRoute('/roster')({
  validateSearch: z.object({ edit: z.string().optional().catch(undefined) }),
  component: RosterPage,
})

function RosterPage() {
  const { edit } = Route.useSearch()
  const navigate = Route.useNavigate()
  return <RosterGrid editing={edit} onEdit={(address) => void navigate({ search: { edit: address } })} />
}
