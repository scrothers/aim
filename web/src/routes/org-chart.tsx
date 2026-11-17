import { createFileRoute } from '@tanstack/react-router'
import { OrgChart } from '@/components/orgchart/OrgChart'

export const Route = createFileRoute('/org-chart')({
  component: OrgChart,
})
