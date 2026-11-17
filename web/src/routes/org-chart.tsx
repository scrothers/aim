import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/org-chart')({
  component: () => (
    <section aria-label="Org chart" className="box-border flex min-w-0 flex-1 flex-col gap-2 p-6">
      <h1 className="m-0 text-[22px] leading-[30px] font-semibold">Org chart</h1>
      <p className="m-0 text-[13px] text-faint">Arrives in a later step.</p>
    </section>
  ),
})
