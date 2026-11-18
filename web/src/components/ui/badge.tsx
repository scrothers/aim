import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "cn"
import { Slot } from "radix-ui"

// Shared by every count: an 18px pill that fits one to three digits.
const pill = "h-[18px] min-w-5 rounded-full px-1.5 leading-none tabular-nums"

/**
 * One badge system for the console.
 *
 * - `count`: a neutral pill for totals that need nothing from the operator.
 * - `count-attention`: a brand-filled pill for counts that need the operator
 *   (operator unread, pending approvals). Only these use the brand fill.
 * - `attention`: a soft red pill for agents that are blocked or failed.
 * - `status`, `priority`, `health`: task chips; pick the colour with `tone`.
 */
const badgeVariants = cva(
  "inline-flex w-fit shrink-0 items-center justify-center gap-1 overflow-hidden border border-transparent text-xs font-medium whitespace-nowrap transition-[color,box-shadow] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring [&>svg]:pointer-events-none [&>svg]:size-3",
  {
    variants: {
      variant: {
        default: "rounded-full bg-primary px-2 py-0.5 text-primary-foreground [a&]:hover:bg-primary/90",
        secondary: "rounded-full bg-secondary px-2 py-0.5 text-secondary-foreground [a&]:hover:bg-secondary/90",
        outline: "rounded-full border-border px-2 py-0.5 text-foreground [a&]:hover:bg-hover",
        count: cn(pill, "bg-code text-muted-foreground"),
        "count-attention": cn(pill, "bg-brand font-semibold text-on-accent"),
        attention: cn(pill, "bg-attention-soft font-semibold text-attention-text"),
        status: "gap-1.5",
        priority: "h-[18px] rounded px-1.5 text-[11px] leading-none font-semibold tracking-wide uppercase",
        health: "h-[18px] rounded px-1 text-[11px]",
      },
      tone: {
        neutral: "bg-code text-muted-foreground",
        faint: "bg-code text-faint",
        brand: "bg-accent-soft text-accent-text",
        online: "bg-online-soft text-online-text",
        idle: "bg-code text-idle",
        attention: "bg-attention-soft text-attention-text",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
)

type BadgeVariant = NonNullable<VariantProps<typeof badgeVariants>["variant"]>
type BadgeTone = NonNullable<VariantProps<typeof badgeVariants>["tone"]>

function Badge({
  className,
  variant = "default",
  tone,
  asChild = false,
  ...props
}: React.ComponentProps<"span"> &
  VariantProps<typeof badgeVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "span"

  return (
    <Comp
      data-slot="badge"
      data-variant={variant}
      data-tone={tone ?? undefined}
      className={cn(badgeVariants({ variant, tone }), className)}
      {...props}
    />
  )
}

/**
 * A count badge, or nothing when the count is zero or unknown. `attention`
 * picks the brand fill; `label` names the count for screen readers
 * ("3 unread").
 */
function CountBadge({
  count,
  label,
  attention = false,
  className,
}: {
  count: number | undefined
  label: string
  attention?: boolean
  className?: string
}) {
  if (!count) return null
  return (
    <Badge variant={attention ? "count-attention" : "count"} aria-label={`${count} ${label}`} className={className}>
      {count}
    </Badge>
  )
}

export { Badge, CountBadge, badgeVariants, type BadgeTone, type BadgeVariant }
