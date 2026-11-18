import { cn } from "../../lib/utils"

interface LoadingScreenProps {
  message?: string
  className?: string
}

/**
 * Full-screen loader for the app start only (before the shell exists); pages show
 * PageSkeleton while their code loads.
 */
export function LoadingScreen({ message = "Loading...", className }: LoadingScreenProps) {
  return (
    <div
      className={cn("fixed inset-0 z-50 flex flex-col items-center justify-center gap-4 bg-background", className)}
      role="status"
      aria-label={message}
    >
      <div className="size-10 animate-spin rounded-full border-4 border-primary border-t-transparent" />
      <p className="text-sm text-muted-foreground">{message}</p>
    </div>
  )
}
