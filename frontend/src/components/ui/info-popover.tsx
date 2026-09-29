import * as React from "react"
import { Info } from "lucide-react"

import { Popover, PopoverTrigger, PopoverContent } from "./popover"
import { cn } from "../../lib/utils"

interface InfoPopoverProps {
  /** Name of the thing explained; used for the button's accessible label. */
  label: string
  children: React.ReactNode
  align?: "start" | "center" | "end"
  iconClassName?: string
  className?: string
}

// Info icon that opens an explanation on click/tap (works the same for mouse, touch and
// keyboard). The content keeps a gutter to the viewport edges, never grows wider than
// the screen, and hides while its trigger is scrolled out of view (e.g. a swiped
// carousel slide) instead of floating detached over other content.
export function InfoPopover({
  label,
  children,
  align = "center",
  iconClassName = "h-3.5 w-3.5",
  className,
}: InfoPopoverProps): React.ReactElement {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className={cn(
            "-m-1 inline-flex shrink-0 items-center justify-center rounded-sm p-1.5 text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground",
            className
          )}
          aria-label={`Info about ${label}`}
        >
          <Info className={iconClassName} />
        </button>
      </PopoverTrigger>
      <PopoverContent
        align={align}
        sideOffset={8}
        collisionPadding={12}
        hideWhenDetached
        className="w-72 max-w-[calc(100vw-24px)] whitespace-normal break-words text-sm text-popover-foreground"
      >
        {children}
      </PopoverContent>
    </Popover>
  )
}
