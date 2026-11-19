import { ChevronRight } from "lucide-react";
import { useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/libraries/utils";

/**
 * Disclosure is a titled section of a Device's page that opens and closes,
 * for detail most visits don't need.
 */
const Disclosure = ({
  title,
  description,
  defaultOpen = false,
  open: controlledOpen,
  onOpenChange,
  size = "base",
  keepMounted = false,
  heading,
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
  // keepMounted hides the content while closed instead of removing it, so
  // an editor inside keeps its unsaved changes.
  keepMounted?: boolean;
  // heading wraps the toggle in a heading of that level, for a Disclosure
  // that is a section of the page in its own right.
  heading?: "h2" | "h3";
  children: React.ReactNode;
}) => {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen);
  const open = controlledOpen ?? uncontrolledOpen;
  const setOpen = (next: boolean) => {
    if (controlledOpen === undefined) setUncontrolledOpen(next);
    onOpenChange?.(next);
  };
  const id = useId();
  const Heading = heading;
  const toggle = (
    <Button
      aria-controls={id}
      aria-expanded={open}
      className={cn(
        "flex w-full items-center gap-2 text-left font-medium",
        heading
          ? "text-lg font-semibold"
          : size === "base"
            ? "text-base"
            : "text-sm",
      )}
      onClick={() => setOpen(!open)}
      variant="unstyled"
    >
      <ChevronRight
        className={cn("h-4 w-4 transition-transform", open && "rotate-90")}
      />
      {title}
    </Button>
  );

  return (
    <div>
      {Heading ? <Heading>{toggle}</Heading> : toggle}
      {description && (
        <p className="mt-1 text-sm text-muted-foreground">{description}</p>
      )}
      {(open || keepMounted) && (
        <div
          className={cn(size === "base" ? "mt-4 space-y-4" : "mt-3 space-y-3")}
          hidden={!open}
          id={id}
        >
          {children}
        </div>
      )}
    </div>
  );
};

export default Disclosure;
