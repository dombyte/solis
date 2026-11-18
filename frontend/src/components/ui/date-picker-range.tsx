"use client"

import React, { useState, useEffect } from "react"
import { format } from "date-fns"
import { CalendarIcon, Check } from "lucide-react"
import { type DateRange } from "react-day-picker"

import { cn } from "../../lib/utils"
import { Button } from "./button"
import { Calendar } from "./calendar"
import { Field, FieldLabel } from "./field"
import { Popover, PopoverContent, PopoverTrigger } from "./popover"
import { useCompactLayout } from "../../hooks/useMobile"
import type { Period } from "../../types"
import { formatRange } from "../../lib/utils/reportRange"

/** The trigger text in the app's date style: "1 – 18 Nov 2026", "Jan – Nov 2026", "2021 – 2026". */
function rangeLabel(from: Date, to: Date, period: Period): string {
  if (period === 'yearly') {
    const [a, b] = [from.getFullYear(), to.getFullYear()]
    return a === b ? String(a) : `${a} – ${b}`
  }
  if (period === 'monthly') {
    const [a, b] = [format(from, "MMM yyyy"), format(to, "MMM yyyy")]
    if (a === b) return a
    return from.getFullYear() === to.getFullYear() ? `${format(from, "MMM")} – ${b}` : `${a} – ${b}`
  }
  return formatRange({ from: format(from, "yyyy-MM-dd"), to: format(to, "yyyy-MM-dd") })
}

export function DatePickerWithRange({
  date,
  setDate,
  className,
  label = "Date Picker Range",
  period = 'daily',
  hideLabel = false,
}: {
  date: DateRange | undefined
  setDate: (range: DateRange | undefined) => void
  className?: string
  label?: string
  period?: Period
  hideLabel?: boolean
}) {
  const [open, setOpen] = useState(false)

  // Use a ref to track the previous date prop to avoid unnecessary state updates
  const prevDateRef = React.useRef(date)

  // Derived state for tempRange - initialized from date prop
  const [tempRange, setTempRange] = useState<DateRange | undefined>(date)

  // Compact = touch or narrow: two months only fit a wide window with a mouse.
  const isMobile = useCompactLayout();
  const numberOfMonths = isMobile ? 1 : 2

  // Sync tempRange when popover opens - only update if date has changed since last open
  useEffect(() => {
    if (open && prevDateRef.current !== date) {
      prevDateRef.current = date
      setTempRange(date)
    }
  }, [open, date])

  const handleSelect = (range: DateRange | undefined) => {
    setTempRange(range)
  }

  const handleApply = () => {
    if (tempRange?.from && tempRange?.to) {
      setDate(tempRange)
      setOpen(false)
    }
  }

  const isRangeValid = tempRange?.from && tempRange?.to

  return (
    <Field className={`mx-auto w-full ${className}`}>
      {!hideLabel && <FieldLabel htmlFor="date-picker-range">{label}</FieldLabel>}
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            id="date-picker-range"
            className="w-full justify-start text-left font-normal min-h-[44px] touch-target"
          >
            <CalendarIcon className="mr-2 h-4 w-4" />
            {date?.from && date.to ? rangeLabel(date.from, date.to, period) : <span>Pick a date</span>}
          </Button>
        </PopoverTrigger>
        {/* Touch devices show one month as wide as the date field (lined up with it, at
            most the screen minus the 12px collision padding) instead of content width with
            wide margins; the day grid stretches with it. Two months size to their content.
            Capped at the space Radix measured on the chosen side, scrolling inside, so a short
            screen never cuts off the month header or the Apply button. */}
        <PopoverContent
          className={cn(
            "p-0 overflow-y-auto max-h-[var(--radix-popover-content-available-height)]",
            isMobile ? "w-[min(var(--radix-popover-trigger-width),calc(100vw-1.5rem))]" : "w-auto",
          )}
          // Start-aligned: centered under the narrow sidebar field it spilled over the
          // app's navigation sidebar.
          align="start"
          collisionPadding={12}
        >
          <div className={isMobile ? "p-3 w-full" : "p-3 w-full max-w-[600px] min-w-[280px]"}>
            <Calendar
              mode="range"
              defaultMonth={date?.from}
              maxDate={new Date()}
              selected={tempRange}
              onSelect={handleSelect}
              numberOfMonths={numberOfMonths}
              className="w-full"
              viewMode={period}
            />
            <div className="mt-4">
              <Button
                size="sm"
                className="w-full"
                onClick={handleApply}
                disabled={!isRangeValid}
              >
                <Check className="mr-2 h-4 w-4" />
                Apply
              </Button>
            </div>
          </div>
        </PopoverContent>
      </Popover>
    </Field>
  )
}
