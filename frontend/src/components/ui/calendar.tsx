"use client"

import * as React from "react"
import { useState } from "react"
import { ChevronLeft, ChevronRight } from "lucide-react"
import { 
  format, 
  startOfMonth, 
  endOfMonth, 
  startOfWeek, 
  endOfWeek, 
  addDays, 
  subMonths, 
  addMonths, 
  subYears,
  addYears,
  isSameMonth, 
  isSameDay, 
  startOfDay,
} from "date-fns"
import type { DateRange } from "react-day-picker"

const WEEKDAYS = ["Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"]
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

type ViewMode = 'daily' | 'monthly' | 'yearly'

type CalendarMode = 'single' | 'multiple' | 'range' | undefined

interface CalendarProps {
  mode?: CalendarMode
  selected?: DateRange | undefined
  onSelect?: (range: DateRange | undefined) => void
  defaultMonth?: Date
  numberOfMonths?: number
  className?: string
  viewMode?: ViewMode
}

export function Calendar({
  mode = "range",
  selected,
  onSelect,
  defaultMonth = new Date(),
  numberOfMonths = 1,
  className,
  viewMode = 'daily',
}: CalendarProps) {
  const [currentMonth, setCurrentMonth] = useState(defaultMonth)

  const prevMonth = () => setCurrentMonth(subMonths(currentMonth, 1))
  const nextMonth = () => setCurrentMonth(addMonths(currentMonth, 1))
  const prevYear = () => setCurrentMonth(subYears(currentMonth, 1))
  const nextYear = () => setCurrentMonth(addYears(currentMonth, 1))

  const renderMonth = (month: Date) => {
    const monthStart = startOfMonth(month)
    const monthEnd = endOfMonth(month)
    const start = startOfWeek(monthStart, { weekStartsOn: 1 })
    const end = endOfWeek(monthEnd, { weekStartsOn: 1 })

    const rows: Date[][] = []
    let days: Date[] = []
    let day = start

    while (!isSameDay(day, end) || days.length === 0) {
      days.push(day)
      day = addDays(day, 1)
      if (days.length === 7) {
        rows.push(days)
        days = []
      }
    }
    if (days.length > 0) {
      rows.push(days)
    }

    return rows
  }

  const handleMonthClick = (monthIndex: number) => {
    if (!onSelect) return
    
    const newDate = new Date(currentMonth)
    newDate.setMonth(monthIndex)
    
    if (mode === "single") {
      onSelect({ from: newDate, to: new Date(newDate.getFullYear(), newDate.getMonth() + 1, 0) } as DateRange)
    } else if (mode === "range") {
      const range = selected as DateRange | undefined
      if (!range || !range.from) {
        onSelect({ from: newDate, to: undefined } as DateRange)
      } else if (range.from && !range.to) {
        const newToDate = new Date(range.from)
        newToDate.setMonth(monthIndex)
        if (newToDate < range.from) {
          onSelect({ from: newToDate, to: range.from } as DateRange)
        } else {
          onSelect({ from: range.from, to: newToDate } as DateRange)
        }
      } else {
        onSelect({ from: newDate, to: undefined } as DateRange)
      }
    }
  }

  const handleYearClick = (year: number) => {
    if (!onSelect) return
    
    const newFrom = new Date(year, 0, 1)
    const newTo = new Date(year, 11, 31)
    
    if (mode === "single") {
      onSelect({ from: newFrom, to: newTo } as DateRange)
    } else if (mode === "range") {
      const range = selected as DateRange | undefined
      if (!range || !range.from) {
        onSelect({ from: newFrom, to: undefined } as DateRange)
      } else if (range.from && !range.to) {
        const newToYear = new Date(range.from)
        newToYear.setFullYear(year)
        if (newToYear < range.from) {
          onSelect({ from: newToYear, to: range.from } as DateRange)
        } else {
          onSelect({ from: range.from, to: newToYear } as DateRange)
        }
      } else {
        onSelect({ from: newFrom, to: undefined } as DateRange)
      }
    }
  }

  const handleDayClick = (day: Date) => {
    if (!onSelect) return

    if (mode === "single") {
      onSelect({ from: day, to: day } as DateRange)
    } else if (mode === "range") {
      const range = selected as DateRange | undefined
      if (!range || !range.from) {
        onSelect({ from: day, to: undefined } as DateRange)
      } else if (range.from && !range.to) {
        if (day < range.from) {
          onSelect({ from: day, to: range.from } as DateRange)
        } else {
          onSelect({ from: range.from, to: day } as DateRange)
        }
      } else {
        onSelect({ from: day, to: undefined } as DateRange)
      }
    }
  }

  const isInRange = (day: Date) => {
    if (!selected) return false
    const range = selected as DateRange
    if (range.from && range.to && day >= range.from && day <= range.to) {
      return true
    }
    return false
  }

  const monthsToRender: Date[] = []
  for (let i = 0; i < numberOfMonths; i++) {
    monthsToRender.push(addMonths(currentMonth, i))
  }

  // Render yearly view (only years)
  if (viewMode === 'yearly') {
    const currentYear = currentMonth.getFullYear()
    const years = Array.from({ length: 12 }, (_, i) => currentYear - 5 + i)
    
    return (
      <div className={`w-full ${className}`}>
        <div className="flex items-center justify-between mb-4">
          <button
            type="button"
            onClick={prevYear}
            className="p-1 rounded hover:bg-muted/80 dark:hover:bg-muted/80"
          >
            <ChevronLeft className="h-5 w-5" />
          </button>
          <span className="font-medium">{years[0]} - {years[years.length - 1]}</span>
          <button
            type="button"
            onClick={nextYear}
            className="p-1 rounded hover:bg-muted/80 dark:hover:bg-muted/80"
          >
            <ChevronRight className="h-5 w-5" />
          </button>
        </div>
        <div className="grid grid-cols-4 gap-2">
          {years.map((year) => {
            const range = selected as DateRange | undefined
            const isRangeStart = range?.from && range.from.getFullYear() === year
            const isRangeEnd = range?.to && range.to.getFullYear() === year
            const isInRange = range?.from && range?.to && year >= range.from.getFullYear() && year <= range.to.getFullYear()
            const isInSelectedRange = isRangeStart || isRangeEnd || isInRange
            const isCurrentYear = year === new Date().getFullYear()
            
            return (
              <button
                key={year}
                type="button"
                onClick={() => handleYearClick(year)}
                className={`
                  aspect-square w-full rounded text-sm transition-colors font-medium
                  ${isInSelectedRange 
                    ? "bg-primary text-primary-foreground" 
                    : isCurrentYear
                    ? "bg-primary/20 dark:bg-primary/30 ring-2 ring-primary font-bold" 
                    : "bg-transparent hover:bg-muted/80 dark:hover:bg-muted/80"
                  }
                `}
                aria-selected={isInSelectedRange}
              >
                {year}
              </button>
            )
          })}
        </div>
      </div>
    )
  }

  // Render monthly view (only months and years)
  if (viewMode === 'monthly') {
    return (
      <div className={`w-full ${className}`}>
        <div className="flex items-center justify-between mb-4">
          <button
            type="button"
            onClick={prevYear}
            className="p-1 rounded hover:bg-muted/80 dark:hover:bg-muted/80"
          >
            <ChevronLeft className="h-5 w-5" />
          </button>
          <span className="font-medium">{format(currentMonth, "yyyy")}</span>
          <button
            type="button"
            onClick={nextYear}
            className="p-1 rounded hover:bg-muted/80 dark:hover:bg-muted/80"
          >
            <ChevronRight className="h-5 w-5" />
          </button>
        </div>
        <div className="grid grid-cols-3 gap-2">
          {MONTHS.map((monthLabel, monthIndex) => {
            const date = new Date(currentMonth.getFullYear(), monthIndex, 1)
            const range = selected as DateRange | undefined
            const isRangeStart = range?.from && range.from.getMonth() === monthIndex && range.from.getFullYear() === currentMonth.getFullYear()
            const isRangeEnd = range?.to && range.to.getMonth() === monthIndex && range.to.getFullYear() === currentMonth.getFullYear()
            const isInRange = range?.from && range?.to && 
              date >= startOfMonth(range.from) && 
              new Date(currentMonth.getFullYear(), monthIndex + 1, 0) <= endOfMonth(range.to || range.from)
            const isInSelectedRange = isRangeStart || isRangeEnd || isInRange
            const isCurrentMonth = monthIndex === new Date().getMonth() && currentMonth.getFullYear() === new Date().getFullYear()
            
            return (
              <button
                key={monthIndex}
                type="button"
                onClick={() => handleMonthClick(monthIndex)}
                className={`
                  aspect-video w-full rounded text-sm transition-colors font-medium
                  ${isInSelectedRange 
                    ? "bg-primary text-primary-foreground" 
                    : isCurrentMonth
                    ? "bg-primary/20 dark:bg-primary/30 ring-2 ring-primary font-bold" 
                    : "bg-transparent hover:bg-muted/80 dark:hover:bg-muted/80"
                  }
                `}
                aria-selected={isInSelectedRange}
              >
                {monthLabel}
              </button>
            )
          })}
        </div>
      </div>
    )
  }

  // Default daily view (show full calendar with days)
  return (
    <div className={`w-full ${className}`}>
      <div className="flex items-center justify-between mb-4">
        <button
          type="button"
          onClick={prevMonth}
          className="p-1 rounded hover:bg-muted/80 dark:hover:bg-muted/80"
        >
          <ChevronLeft className="h-5 w-5" />
        </button>
        <button
          type="button"
          onClick={nextMonth}
          className="p-1 rounded hover:bg-muted/80 dark:hover:bg-muted/80"
        >
          <ChevronRight className="h-5 w-5" />
        </button>
      </div>

      <div className="flex gap-4">
        {monthsToRender.map((month) => (
          <div key={month.toString()} className="w-full">
            {/* Month label above each calendar */}
            <div className="text-center font-medium mb-2">
              {format(month, "MMMM yyyy")}
            </div>
            {/* Weekday headers */}
            <div className="grid grid-cols-7 gap-1 mb-1">
              {WEEKDAYS.map((day) => (
                <div
                  key={day}
                  className="text-center text-sm font-medium text-muted-foreground py-2"
                >
                  {day}
                </div>
              ))}
            </div>

            {/* Days grid */}
            <div className="grid grid-cols-7 gap-1">
              {renderMonth(month).map((week, weekIndex) => (
                <React.Fragment key={weekIndex}>
                  {week.map((day) => {
                    const isCurrentMonth = isSameMonth(day, month)
                    const isToday = isSameDay(day, startOfDay(new Date()))
                    const range = selected as DateRange | undefined
                    const isRangeStart = range?.from && isSameDay(day, range.from)
                    const isRangeEnd = range?.to && isSameDay(day, range.to)
                    const inRange = isInRange(day)
                    const isInSelectedRange = inRange || isRangeStart || isRangeEnd

                    return (
                      <button
                        key={day.toString()}
                        type="button"
                        onClick={() => handleDayClick(day)}
                        disabled={!isCurrentMonth}
                        className={`
                          aspect-square w-full rounded text-sm transition-colors font-medium
                          ${isCurrentMonth 
                            ? "cursor-pointer" 
                            : "text-muted-foreground cursor-not-allowed"
                          }
                          ${isRangeStart 
                            ? "bg-primary text-primary-foreground rounded-l-md" 
                            : isRangeEnd 
                            ? "bg-primary text-primary-foreground rounded-r-md" 
                            : isInSelectedRange 
                            ? "bg-primary/70 dark:bg-primary/80 text-primary-foreground" 
                            : isToday 
                            ? "bg-primary/20 dark:bg-primary/30 ring-2 ring-primary font-bold" 
                            : "bg-transparent hover:bg-muted/80 dark:hover:bg-muted/80"
                          }
                        `}
                        aria-selected={isInSelectedRange}
                      >
                        {format(day, "d")}
                      </button>
                    )
                  })}
                  {/* Line break after each week (7 items) */}
                  {weekIndex < renderMonth(month).length - 1 && (
                    <div className="col-span-7 h-px bg-muted/50 dark:bg-muted/50 my-1" />
                  )}
                </React.Fragment>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

Calendar.displayName = "Calendar"
