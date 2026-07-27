"use client"

import React, { useState, useEffect } from "react"
import { addDays, format } from "date-fns"
import { CalendarIcon, Check } from "lucide-react"
import { type DateRange } from "react-day-picker"

import { Button } from "./button"
import { Calendar } from "./calendar"
import { Field, FieldLabel } from "./field"
import { Popover, PopoverContent, PopoverTrigger } from "./popover"
import type { Period } from "../../types"

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
  const [numberOfMonths, setNumberOfMonths] = useState(2)
  
  // Use a ref to track the previous date prop to avoid unnecessary state updates
  const prevDateRef = React.useRef(date)
  
  // Derived state for tempRange - initialized from date prop
  const [tempRange, setTempRange] = useState<DateRange | undefined>(date)

  useEffect(() => {
    const handleResize = () => {
      setNumberOfMonths(window.innerWidth >= 768 ? 2 : 1)
    }
    handleResize()
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

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

  const displayDate = date || {
    from: new Date(new Date().getFullYear(), 0, 20),
    to: addDays(new Date(new Date().getFullYear(), 0, 20), 20),
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
            {displayDate?.from ? (
              displayDate.to ? (
                <>
                  {period === 'yearly' ? (
                    `${displayDate.from.getFullYear()} - ${displayDate.to.getFullYear()}`
                  ) : period === 'monthly' ? (
                    `${format(displayDate.from, "yyyy-MM")} - ${format(displayDate.to, "yyyy-MM")}`
                  ) : (
                    <>
                      {format(displayDate.from, "LLL dd, y")} - {" "}
                      {format(displayDate.to, "LLL dd, y")}
                    </>
                  )}
                </>
              ) : (
                period === 'yearly' ? (
                  String(displayDate.from.getFullYear())
                ) : period === 'monthly' ? (
                  format(displayDate.from, "yyyy-MM")
                ) : (
                  format(displayDate.from, "LLL dd, y")
                )
              )
            ) : (
              <span>Pick a date</span>
            )}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-auto p-0" align="center">
          <div className="p-3 w-full max-w-[600px] min-w-[280px]">
            <Calendar
              mode="range"
              defaultMonth={displayDate?.from}
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
