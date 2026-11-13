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
  size = "base",
  children,
}: {
  title: string;
  description?: string;
  defaultOpen?: boolean;
  // base is a connection's own heading size and spacing; sm sits inside a
  // connection.
  size?: "base" | "sm";
  children: React.ReactNode;
}) => {
  const [open, setOpen] = useState(defaultOpen);
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
        onClick={() => setOpen((value) => !value)}
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
