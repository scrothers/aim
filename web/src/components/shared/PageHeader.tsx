import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

/** Title sizes on one scale: a page title and the smaller title of a pane inside a page. */
export const TITLE_CLS = {
  page: 'text-[22px] leading-[30px] font-semibold',
  column: 'text-lg leading-7 font-semibold',
} as const

export interface PageHeaderProps {
  title: ReactNode
  /** One line under the title. */
  description?: ReactNode
  /** Shown right after the title, e.g. a count badge. */
  badge?: ReactNode
  /**
   * Right-side controls. Use default-size buttons: the primary action as
   * `default`, everything else as `outline`.
   */
  actions?: ReactNode
  /** `column` for a pane inside a page (the mail list); `page` otherwise. */
  size?: keyof typeof TITLE_CLS
  /** Heading level; panes under a page h1 use h2. */
  as?: 'h1' | 'h2'
  className?: string
  children?: ReactNode
}

/** The header every page uses: title, optional description, actions on the right. */
export function PageHeader({ title, description, badge, actions, size = 'page', as: Heading = 'h1', className, children }: PageHeaderProps) {
  return (
    <header data-slot="page-header" data-size={size} className={cn('flex flex-col gap-1', className)}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <div className="mr-auto flex min-w-32 flex-1 items-center gap-2">
          <Heading className={cn('m-0 min-w-0 truncate', TITLE_CLS[size])}>{title}</Heading>
          {badge}
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </div>
      {description && <p className="m-0 text-[13px] text-muted-foreground">{description}</p>}
      {children}
    </header>
  )
}
