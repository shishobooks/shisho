import { Check } from "lucide-react";
import type { ReactNode } from "react";

import Logo from "@/components/library/Logo";
import { Button } from "@/components/ui/button";
import { cn } from "@/libraries/utils";

export type SetupStepKey = "account" | "library" | "plugins" | "finish";

const STEPS: { key: SetupStepKey; label: string }[] = [
  { key: "account", label: "Account" },
  { key: "library", label: "Library" },
  { key: "plugins", label: "Plugins" },
  { key: "finish", label: "Finish" },
];

const Stepper = ({ current }: { current: SetupStepKey }) => {
  const currentIndex = STEPS.findIndex((s) => s.key === current);
  return (
    <ol aria-label="Setup steps" className="flex items-center gap-1.5 sm:gap-2">
      {STEPS.map((step, i) => {
        const done = i < currentIndex;
        const isCurrent = i === currentIndex;
        return (
          <li
            aria-current={isCurrent ? "step" : undefined}
            className="flex items-center gap-1.5 sm:gap-2"
            key={step.key}
          >
            {i > 0 && (
              <span
                aria-hidden="true"
                className={cn(
                  "h-px w-4 sm:w-8",
                  done || isCurrent ? "bg-primary" : "bg-border",
                )}
              />
            )}
            <span
              aria-hidden="true"
              className={cn(
                "flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-xs font-medium",
                done && "border-primary bg-primary text-primary-foreground",
                isCurrent && "border-primary text-primary",
                !done && !isCurrent && "border-border text-muted-foreground",
              )}
            >
              {done ? <Check className="h-3.5 w-3.5" /> : i + 1}
            </span>
            {/* Phones show only the current step's name, so the row fits. */}
            <span
              className={cn(
                "text-sm",
                isCurrent
                  ? "font-medium text-foreground"
                  : "sr-only text-muted-foreground sm:not-sr-only",
              )}
            >
              {step.label}
              {done && <span className="sr-only"> (done)</span>}
            </span>
          </li>
        );
      })}
    </ol>
  );
};

interface SetupLayoutProps {
  step: SetupStepKey;
  title: string;
  description?: ReactNode;
  /** Shows the quiet "Skip setup" link. */
  onSkip?: () => void;
  /** The account form is narrower than the later steps. */
  narrow?: boolean;
  children: ReactNode;
}

/**
 * The frame every Setup step shares: the logo, where the admin is in the
 * flow, the step's heading, and on the steps after the account a quiet way
 * out.
 */
export const SetupLayout = ({
  step,
  title,
  description,
  onSkip,
  narrow,
  children,
}: SetupLayoutProps) => (
  <div className="min-h-[calc(100vh-var(--banner-height,0px))] bg-background">
    <div
      className={cn(
        "mx-auto w-full px-4 py-8 md:py-12",
        narrow ? "max-w-md" : "max-w-2xl",
      )}
    >
      <div className="mb-6 flex flex-col items-center gap-5 md:mb-8">
        <Logo size="lg" />
        <Stepper current={step} />
      </div>
      <div className="mb-6 text-center">
        <h1 className="mb-1 text-xl font-semibold md:text-2xl">{title}</h1>
        {description && <p className="text-muted-foreground">{description}</p>}
      </div>
      {children}
      {onSkip && (
        <div className="mt-8 flex justify-center">
          <Button
            className="text-muted-foreground"
            onClick={onSkip}
            size="sm"
            variant="link"
          >
            Skip setup
          </Button>
        </div>
      )}
    </div>
  </div>
);
