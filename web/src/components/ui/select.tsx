import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "cn"
import { CheckIcon, ChevronDownIcon, ChevronUpIcon } from "lucide-react"
import { Select as SelectPrimitive } from "radix-ui"

function Select(props: React.ComponentProps<typeof SelectPrimitive.Root>) {
  return <SelectPrimitive.Root data-slot="select" {...props} />
}

function SelectGroup(props: React.ComponentProps<typeof SelectPrimitive.Group>) {
  return <SelectPrimitive.Group data-slot="select-group" {...props} />
}

function SelectValue(props: React.ComponentProps<typeof SelectPrimitive.Value>) {
  return <SelectPrimitive.Value data-slot="select-value" {...props} />
}

/** Same box as Input: border, height, text and placeholder colours, focus ring. */
const selectTriggerVariants = cva(
  "flex w-full min-w-0 items-center justify-between gap-2 rounded-md border border-input bg-transparent text-left font-normal text-text shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive data-[placeholder]:text-faint dark:bg-input/30 *:data-[slot=select-value]:flex *:data-[slot=select-value]:min-w-0 *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-2 *:data-[slot=select-value]:truncate [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      size: {
        default: "h-9 px-3 text-sm",
        sm: "h-8 px-2.5 text-[13px]",
      },
    },
    defaultVariants: { size: "default" },
  }
)

function SelectTrigger({
  className,
  size = "default",
  children,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Trigger> & VariantProps<typeof selectTriggerVariants>) {
  return (
    <SelectPrimitive.Trigger
      data-slot="select-trigger"
      data-size={size}
      className={cn(selectTriggerVariants({ size }), className)}
      {...props}
    >
      {children}
      <SelectPrimitive.Icon asChild>
        <ChevronDownIcon className="size-4 text-faint" />
      </SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
  )
}

function SelectContent({
  className,
  children,
  position = "popper",
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Content>) {
  return (
    <SelectPrimitive.Portal>
      <SelectPrimitive.Content
        data-slot="select-content"
        className={cn(
          "relative z-50 max-h-(--radix-select-content-available-height) min-w-[8rem] origin-(--radix-select-content-transform-origin) overflow-x-hidden overflow-y-auto rounded-md border bg-popover text-popover-foreground shadow-md data-[side=bottom]:slide-in-from-top-2 data-[side=top]:slide-in-from-bottom-2 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0",
          position === "popper" && "data-[side=bottom]:translate-y-1 data-[side=top]:-translate-y-1",
          className
        )}
        position={position}
        {...props}
      >
        <SelectPrimitive.ScrollUpButton className="flex cursor-default items-center justify-center py-1">
          <ChevronUpIcon className="size-4" />
        </SelectPrimitive.ScrollUpButton>
        <SelectPrimitive.Viewport
          className={cn(
            "p-1",
            position === "popper" && "w-full min-w-(--radix-select-trigger-width) scroll-my-1"
          )}
        >
          {children}
        </SelectPrimitive.Viewport>
        <SelectPrimitive.ScrollDownButton className="flex cursor-default items-center justify-center py-1">
          <ChevronDownIcon className="size-4" />
        </SelectPrimitive.ScrollDownButton>
      </SelectPrimitive.Content>
    </SelectPrimitive.Portal>
  )
}

function SelectItem({ className, children, ...props }: React.ComponentProps<typeof SelectPrimitive.Item>) {
  return (
    <SelectPrimitive.Item
      data-slot="select-item"
      className={cn(
        "relative flex w-full cursor-default items-center gap-2 rounded-sm py-1.5 pr-8 pl-2 text-sm text-text outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 max-md:min-h-11 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        className
      )}
      {...props}
    >
      <span className="absolute right-2 flex size-3.5 items-center justify-center">
        <SelectPrimitive.ItemIndicator>
          <CheckIcon className="size-4" />
        </SelectPrimitive.ItemIndicator>
      </span>
      <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
    </SelectPrimitive.Item>
  )
}

function SelectSeparator({ className, ...props }: React.ComponentProps<typeof SelectPrimitive.Separator>) {
  return (
    <SelectPrimitive.Separator
      data-slot="select-separator"
      className={cn("pointer-events-none -mx-1 my-1 h-px bg-border", className)}
      {...props}
    />
  )
}

// Radix reserves "" for "no value", so an option meaning "any" travels as this token.
const EMPTY = "__empty__"

export interface SelectOption {
  value: string
  label: React.ReactNode
}

/**
 * A ready-made single select: options in, string value out. An option with
 * value "" works like a native "All" option.
 */
function SelectField({
  value,
  onValueChange,
  options,
  placeholder,
  size,
  className,
  disabled,
  id,
  ...aria
}: {
  value: string
  onValueChange: (value: string) => void
  options: ReadonlyArray<SelectOption>
  placeholder?: string
  size?: VariantProps<typeof selectTriggerVariants>["size"]
  className?: string
  disabled?: boolean
  id?: string
  "aria-label"?: string
  "aria-labelledby"?: string
  "aria-invalid"?: boolean
}) {
  return (
    <Select value={value === "" ? EMPTY : value} onValueChange={(v) => onValueChange(v === EMPTY ? "" : v)} disabled={disabled}>
      <SelectTrigger id={id} size={size} className={className} {...aria}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.value || EMPTY} value={o.value || EMPTY}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export {
  Select,
  SelectContent,
  SelectField,
  SelectGroup,
  SelectItem,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
  selectTriggerVariants,
}
