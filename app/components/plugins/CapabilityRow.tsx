import { type LucideIcon } from "lucide-react";

export interface CapabilityRowProps {
  description: string;
  detail?: string;
  icon: LucideIcon;
  label: string;
}

export const CapabilityRow = ({
  description,
  detail,
  icon: Icon,
  label,
}: CapabilityRowProps) => (
  <div className="flex items-start gap-3 rounded-md border border-border p-3">
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
