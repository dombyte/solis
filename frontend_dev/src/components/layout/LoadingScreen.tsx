"use client"

import { cn } from "../../lib/utils"

interface LoadingScreenProps {
  message?: string
  fullPage?: boolean
  className?: string
}

export function LoadingScreen({
  message = "Loading...",
  fullPage = true,
  className,
}: LoadingScreenProps) {
  return (
    <div
      className={cn(
        fullPage ? "fixed inset-0 z-50 flex flex-col items-center justify-center" : "absolute inset-0 z-10 flex flex-col items-center justify-center",
        className
      )}
    >
      {/* Backdrop for full page mode - behind content */}
      {fullPage && (
        <div className="absolute inset-0 bg-background/80 backdrop-blur-sm" />
      )}
      
      {/* Content wrapper */}
      <div className="flex flex-col items-center justify-center">
        {/* Spinner */}
        <div className="relative z-10">
          <div className="animate-spin h-12 w-12 border-4 border-primary border-t-transparent rounded-full" />
          <div className="absolute inset-0 animate-spin border-4 border-primary/50 border-t-transparent rounded-full" style={{ animationDelay: '0.15s' }} />
          <div className="absolute inset-0 animate-spin border-4 border-primary/25 border-t-transparent rounded-full" style={{ animationDelay: '0.3s' }} />
        </div>
        
        {/* Loading text */}
        <p className="relative z-10 text-muted-foreground text-center text-sm sm:text-base animate-pulse mt-4">
          {message}
        </p>
      </div>
    </div>
  )
}

// Smaller loading spinner for inline use
export function LoadingSpinner({ className }: { className?: string }) {
  return (
    <div className={cn("flex items-center justify-center", className)}>
      <div className="animate-spin h-6 w-6 border-3 border-primary border-t-transparent rounded-full" />
    </div>
  )
}
