import * as React from "react"
import * as SheetPrimitive from "@radix-ui/react-dialog"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"
import { X } from "lucide-react"

const sheetVariants = cva(
  "fixed z-50 gap-4 bg-background p-6 shadow-lg transition ease-in-out data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[side=right]:slide-in-from-right-1/2 data-[side=right]:sm:slide-in-from-right-1/2 data-[side=right]:sm:max-w-md data-[side=right]:slide-in-from-right-1/2 data-[side=left]:slide-in-from-left-1/2 data-[side=left]:sm:slide-in-from-left-1/2 data-[side=left]:sm:max-w-md data-[side=left]:slide-in-from-left-1/2 data-[side=top]:slide-in-from-top-1/2 data-[side=top]:sm:max-w-md data-[side=top]:slide-in-from-top-1/2 data-[side=bottom]:slide-in-from-bottom-1/2 data-[side=bottom]:sm:max-w-md data-[side=bottom]:slide-in-from-bottom-1/2",
  {
    variants: {
      side: {
        top: "inset-x-0 top-0 border-b data-[state=closed]:slide-out-to-top-1/2 data-[state=closed]:sm:slide-out-to-top-1/2",
        bottom: "inset-x-0 bottom-0 border-t data-[state=closed]:slide-out-to-bottom-1/2 data-[state=closed]:sm:slide-out-to-bottom-1/2",
        left: "inset-y-0 left-0 h-full w-3/4 border-r data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:sm:slide-out-to-left-1/2 sm:max-w-md",
        right: "inset-y-0 right-0 h-full w-3/4 border-l data-[state=closed]:slide-out-to-right-1/2 data-[state=closed]:sm:slide-out-to-right-1/2 sm:max-w-md",
      },
    },
    defaultVariants: {
      side: "right",
    },
  }
)

interface SheetProps {
  side?: "top" | "right" | "bottom" | "left"
  className?: string
}

const Sheet = SheetPrimitive.Root

const SheetTrigger = SheetPrimitive.Trigger

const SheetContent = React.forwardRef<
  React.ElementRef<typeof SheetPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof SheetPrimitive.Content> & VariantProps<typeof sheetVariants> & SheetProps
>(({ side = "right", className, children, ...props }, ref) => (
  <SheetPrimitive.Portal>
    <SheetPrimitive.Overlay
      className={cn(
        "fixed inset-0 z-50 bg-black/80 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
      )}
    />
    <SheetPrimitive.Content
      ref={ref}
      className={cn(sheetVariants({ side }), className)}
      {...props}
    >
      {children}
      {/* 40px touch target centred on a one-line p-4 header (16 + 28px title + 16 = 60px);
          z-20 keeps it above a sticky header. */}
      <SheetPrimitive.Close className="absolute right-2.5 top-2.5 z-20 flex size-10 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-control-hover hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none">
        <X className="size-5" />
        <span className="sr-only">Close</span>
      </SheetPrimitive.Close>
    </SheetPrimitive.Content>
  </SheetPrimitive.Portal>
))
SheetContent.displayName = SheetPrimitive.Content.displayName

const SheetHeader = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn("flex flex-col space-y-2 text-center sm:text-left", className)}
    {...props}
  />
)
SheetHeader.displayName = "SheetHeader"

const SheetTitle = React.forwardRef<
  React.ElementRef<typeof SheetPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof SheetPrimitive.Title>
>(({ className, ...props }, ref) => (
  <SheetPrimitive.Title
    ref={ref}
    className={cn("text-lg font-semibold text-foreground", className)}
    {...props}
  />
))
SheetTitle.displayName = SheetPrimitive.Title.displayName

export {
  Sheet,
  SheetTrigger,
  SheetContent,
  SheetHeader,
  SheetTitle,
}
