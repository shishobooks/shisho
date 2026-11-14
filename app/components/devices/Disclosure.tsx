import { ChevronRight } from "lucide-react";
import { useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/libraries/utils";

/**
 * Disclosure is a titled section of a Device's connections that opens and
 * closes, for detail most visits don't need.
 */
const Disclosure = ({
  title,
  description,
  defaultOpen = false,
  open: controlledOpen,
  onOpenChange,
  size = "base",
  children,
}: {
  title: string;
  description?: string;
  defaultOpen?: boolean;
  // open and onOpenChange let the parent open or close it, as for a
  // Disclosure that opens itself when its content changes.
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  // base is a connection's own heading size and spacing; sm sits inside a
  // connection.
  size?: "base" | "sm";
  children: React.ReactNode;
}) => {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen);
  const open = controlledOpen ?? uncontrolledOpen;
  const setOpen = (next: boolean) => {
    if (controlledOpen === undefined) setUncontrolledOpen(next);
    onOpenChange?.(next);
  };
  const id = useId();

  return (
    <div>
      <Button
        aria-controls={id}
        aria-expanded={open}
        className={cn(
          "flex w-full items-center gap-2 text-left font-medium",
          size === "base" ? "text-base" : "text-sm",
        )}
        onClick={() => setOpen(!open)}
        variant="unstyled"
      >
        <ChevronRight
          className={cn("h-4 w-4 transition-transform", open && "rotate-90")}
        />
        {title}
      </Button>
      {description && (
        <p className="mt-1 text-sm text-muted-foreground">{description}</p>
      )}
      {open && (
        <div
          className={cn(size === "base" ? "mt-4 space-y-4" : "mt-3 space-y-3")}
          id={id}
        >
          {children}
        </div>
      )}
    </div>
  );
};

export default Disclosure;
