import { useQuery } from '@tanstack/react-query'
import { renderQuery } from '@/api/mail'
import { stripMarkdown } from '@/lib/markdown'

/**
 * A role description is Markdown. The server render is sanitized HTML, so it is
 * safe to inject; until it arrives (or if it fails) the raw text shows as plain
 * text, never as HTML.
 */
export function RoleDescription({ markdown }: { markdown: string }) {
  const q = useQuery(renderQuery(markdown))
  const html = q.data?.html
  if (html) {
    return (
      <div
        data-testid="role-description"
        className="prose-aim text-[13px] leading-5 text-muted-foreground"
        // html is sanitized server-side by /api/render.
        dangerouslySetInnerHTML={{ __html: html }}
      />
    )
  }
  return (
    <p data-testid="role-description" className="m-0 text-[13px] leading-5 whitespace-pre-wrap text-muted-foreground">
      {q.isPending ? stripMarkdown(markdown) : markdown}
    </p>
  )
}
