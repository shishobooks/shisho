import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import * as React from "react";

import { FormDialogContext } from "@/components/ui/form-dialog";
import { cn } from "@/libraries/utils";

// Custom Dialog wrapper that fixes Radix pointer-events bug.
// When a dialog triggered from a dropdown menu closes, Radix's DismissableLayer
// incorrectly sets pointer-events: none on the body during unmount.
// This wrapper cleans up pointer-events after the dialog closes or unmounts.
const Dialog = ({
  open,
  onOpenChange,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Root>) => {
  // Clean up pointer-events when dialog closes
  React.useEffect(() => {
    if (open === false) {
      const timeout = setTimeout(() => {
        document.body.style.pointerEvents = "";
      }, 300);
      return () => clearTimeout(timeout);
    }
  }, [open]);

  // Also clean up on unmount (handles conditional rendering case)
  React.useEffect(() => {
    return () => {
      // Use setTimeout to run after Radix's unmount effects
      setTimeout(() => {
        document.body.style.pointerEvents = "";
      }, 300);
    };
  }, []);

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange} {...props} />
  );
};

const DialogTrigger = DialogPrimitive.Trigger;

const DialogPortal = DialogPrimitive.Portal;

const DialogClose = DialogPrimitive.Close;

// DialogContent renders a hidden DialogPrimitive.Close and exposes its ref via
// this context. DialogHeader's visible close button forwards clicks to that
// hidden close button. This keeps DialogHeader renderable outside a Dialog
// (e.g. in unit tests) — the context will be null and the visible button is
// simply omitted instead of throwing from Radix's internal context check.
const DialogCloseRefContext =
  React.createContext<React.RefObject<HTMLButtonElement | null> | null>(null);

const DialogOverlay = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Overlay>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Overlay
    ref={ref}
    className={cn(
      "fixed inset-0 z-50 bg-black/80  data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
      className,
    )}
    {...props}
  />
));
DialogOverlay.displayName = DialogPrimitive.Overlay.displayName;

interface DialogContentProps extends React.ComponentPropsWithoutRef<
  typeof DialogPrimitive.Content
> {
  /**
   * When true, dialog slides up from bottom on mobile for better touch interaction.
   * On desktop (sm+), it renders as a centered modal regardless.
   */
  mobileSheet?: boolean;
}

const DialogContent = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Content>,
  DialogContentProps
>(({ className, children, mobileSheet = false, ...props }, ref) => {
  const closeRef = React.useRef<HTMLButtonElement>(null);
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        ref={ref}
        className={cn(
          // Base styles — flex column so Header/Footer stay sticky and only the
          // Body scrolls. Content's overflow-hidden + each child's shrink behavior
          // means there's no overscroll on the Header/Footer.
          "fixed z-50 flex w-full flex-col overflow-hidden border bg-background shadow-lg duration-200",
          // Animation base
          "data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
          // Mobile sheet variant
          mobileSheet
            ? // Mobile: slide up from bottom, desktop: centered modal
              "inset-x-0 bottom-0 rounded-t-lg max-h-[90vh] data-[state=closed]:slide-out-to-bottom data-[state=open]:slide-in-from-bottom sm:inset-auto sm:left-[50%] sm:top-[50%] sm:translate-x-[-50%] sm:translate-y-[-50%] sm:rounded-lg sm:max-w-lg sm:max-h-[85vh] sm:data-[state=closed]:slide-out-to-left-1/2 sm:data-[state=closed]:slide-out-to-top-[48%] sm:data-[state=open]:slide-in-from-left-1/2 sm:data-[state=open]:slide-in-from-top-[48%] sm:data-[state=closed]:zoom-out-95 sm:data-[state=open]:zoom-in-95"
            : // Default: centered modal
              "left-[50%] top-[50%] translate-x-[-50%] translate-y-[-50%] max-w-lg max-h-[90vh] data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%] data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%] sm:rounded-lg",
          className,
        )}
        {...props}
      >
        <DialogCloseRefContext.Provider value={closeRef}>
          {children}
        </DialogCloseRefContext.Provider>
        {/* Hidden close target — the visible X lives in DialogHeader and forwards
            clicks here so Radix's close behavior runs without DialogHeader needing
            to consume Radix's internal context. */}
        <DialogPrimitive.Close
          aria-hidden
          className="hidden"
          ref={closeRef}
          tabIndex={-1}
        />
      </DialogPrimitive.Content>
    </DialogPortal>
  );
});
DialogContent.displayName = DialogPrimitive.Content.displayName;

// Header is a distinct elevated band that frames the dialog. The close button
// is rendered as a child so it can vertically center inside the band regardless
// of how many lines the header content takes.
const DialogHeader = ({
  className,
  children,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) => {
  // FormDialog bypasses the unsaved-changes warning when closed via the X.
  const formDialogContext = React.useContext(FormDialogContext);
  // Forward clicks to DialogContent's hidden DialogPrimitive.Close. Null when
  // DialogHeader is rendered outside a Dialog (tests render the form bare).
  const closeRef = React.useContext(DialogCloseRefContext);

  const handleClose = formDialogContext
    ? formDialogContext.closeDirectly
    : closeRef
      ? () => closeRef.current?.click()
      : null;

  return (
    <div
      className={cn(
        "relative flex shrink-0 flex-col border-b bg-muted px-5 py-4 pr-10 text-left",
        className,
      )}
      {...props}
    >
      {children}
      {handleClose && (
        <button
          className="absolute right-3 top-1/2 -translate-y-1/2 rounded-sm opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 disabled:pointer-events-none cursor-pointer"
          onClick={handleClose}
          type="button"
        >
          <X className="h-4 w-4" />
          <span className="sr-only">Close</span>
        </button>
      )}
    </div>
  );
};
DialogHeader.displayName = "DialogHeader";

// Body is the padded content slot between header and footer. flex-1 + min-h-0
// lets it absorb leftover height in DialogContent's flex column and scroll
// internally while the banded header/footer stay pinned.
const DialogBody = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn("min-h-0 flex-1 overflow-y-auto px-5 py-4", className)}
    {...props}
  />
);
DialogBody.displayName = "DialogBody";

// Footer mirrors the header's elevated band styling.
const DialogFooter = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn(
      "flex shrink-0 flex-col-reverse gap-2 border-t bg-muted px-5 py-4 sm:flex-row sm:justify-end",
      className,
    )}
    {...props}
  />
);
DialogFooter.displayName = "DialogFooter";

const DialogTitle = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Title>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Title
    ref={ref}
    className={cn("text-sm font-semibold tracking-tight", className)}
    {...props}
  />
));
DialogTitle.displayName = DialogPrimitive.Title.displayName;

const DialogDescription = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Description>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Description>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Description
    ref={ref}
    className={cn("text-xs text-muted-foreground", className)}
    {...props}
  />
));
DialogDescription.displayName = DialogPrimitive.Description.displayName;

export {
  Dialog,
  DialogPortal,
  DialogOverlay,
  DialogTrigger,
  DialogClose,
  DialogContent,
  DialogHeader,
  DialogBody,
  DialogFooter,
  DialogTitle,
  DialogDescription,
};
