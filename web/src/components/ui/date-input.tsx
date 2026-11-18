import * as React from "react"
import { cn } from "cn"
import { CalendarDays } from "lucide-react"
import { Input } from "./input"

/**
 * A date field styled like Input. It stays a native `<input type="date">`
 * for keyboard entry, screen readers and phone pickers, but drops the OS
 * look: a themed calendar icon replaces the browser's, and an empty field
 * shows its "mm/dd/yyyy" mask in the placeholder colour.
 */
function DateInput({
  className,
  wrapperClassName,
  value,
  ...props
}: Omit<React.ComponentProps<"input">, "type" | "value"> & { value: string; wrapperClassName?: string }) {
  const empty = value === ""
  return (
    <span data-slot="date-input" className={cn("relative flex w-full min-w-0 items-center", wrapperClassName)}>
      <Input
        type="date"
        value={value}
        data-empty={empty || undefined}
        className={cn(
          "relative pr-9",
          // The browser's picker button stays clickable but invisible, over our icon.
          "[&::-webkit-calendar-picker-indicator]:absolute [&::-webkit-calendar-picker-indicator]:right-2 [&::-webkit-calendar-picker-indicator]:size-6 [&::-webkit-calendar-picker-indicator]:cursor-pointer [&::-webkit-calendar-picker-indicator]:opacity-0",
          "data-[empty]:[&::-webkit-datetime-edit]:text-faint",
          className
        )}
        {...props}
      />
      {/* Firefox draws its own themed button and ignores the indicator rules above. */}
      <CalendarDays
        aria-hidden="true"
        className="pointer-events-none absolute right-3 size-4 text-faint supports-[-moz-appearance:none]:hidden"
      />
    </span>
  )
}

export { DateInput }
