import { type LucideIcon } from "lucide-react";

import { cn } from "@/libraries/utils";

export interface CapabilityRowProps {
  description: string;
  detail?: string;
  // When true, render as a flat row (no border/rounded/padding) suitable for
  // use inside an outer bordered section card. When false (default), render
  // as a standalone card — used by the install dialog where rows stand alone.
  flat?: boolean;
  icon: LucideIcon;
  label: string;
}

export const CapabilityRow = ({
  description,
  detail,
  flat,
  icon: Icon,
  label,
}: CapabilityRowProps) => (
  <div
    className={cn(
      "flex items-start gap-3",
      flat ? "py-3" : "rounded-md border border-border p-3",
    )}
  >
    <Icon className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
    <div>
      <p className="text-sm font-medium">{label}</p>
      <p className="text-xs text-muted-foreground">{description}</p>
      {detail && (
        <p className="mt-1 font-mono text-xs text-muted-foreground/70">
          {detail}
        </p>
      )}
    </div>
  </div>
);
