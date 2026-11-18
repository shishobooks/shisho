import { CircleCheck, CircleDashed, PartyPopper } from "lucide-react";
import { forwardRef, useEffect, useId, useRef, type RefObject } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import { GETTING_STARTED_HIDDEN_TOAST_ID } from "@/components/layout/gettingStartedToast";
import { Button, type ButtonProps } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { useGettingStartedPanel } from "@/contexts/GettingStarted";
import {
  useClearGettingStartedAutoOpen,
  useDismissGettingStarted,
  useGettingStarted,
  useSetGettingStartedNotForMe,
} from "@/hooks/queries/gettingStarted";
import { useIsPhone } from "@/hooks/useIsPhone";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type {
  GettingStartedResponse,
  GettingStartedStep,
  GettingStartedStepKey,
} from "@/types/generated/gettingstarted";

interface StepCopy {
  title: string;
  description: string;
  to: string;
}

// Where each step gets done. Later Setup, Invite Link, and Connect a device
// work replaces some of these targets.
const STEP_COPY: Record<GettingStartedStepKey, StepCopy> = {
  library: {
    title: "Set up your first library",
    description: "Point Shisho at the folders that hold your books.",
    to: "/libraries/create",
  },
  plugins: {
    title: "Add metadata plugins",
    description: "Plugins fill in covers, descriptions, and series.",
    to: "/admin/plugins/discover",
  },
  invite: {
    title: "Invite someone",
    description: "Give someone an account to share your library with.",
    to: "/admin/users/create",
  },
  device: {
    title: "Connect a reading device",
    description: "Read on your e-reader or a reading app on your phone.",
    to: "/user/devices",
  },
  to_read: {
    title: "Add a book to To Read",
    description: "Line up what you want to read next from any book's page.",
    to: "/",
  },
  start_reading: {
    title: "Start reading something",
    description: "Open a book in your browser or on a connected device.",
    to: "/",
  },
};

interface Progress {
  /** The steps that apply to the user's role, in panel order. */
  steps: GettingStartedStep[];
  /** Steps that count: those that apply, less any skipped and not done. */
  total: number;
  done: number;
  complete: boolean;
}

const progressOf = (data: GettingStartedResponse): Progress => {
  const steps = data.steps.filter((s) => s.applies);
  const counted = steps.filter((s) => s.done || !s.not_for_me);
  const done = counted.filter((s) => s.done).length;
  return {
    steps,
    total: counted.length,
    done,
    complete: steps.length > 0 && done === counted.length,
  };
};

const progressLabel = ({ done, total }: Progress) => `${done} of ${total} done`;

// When every step is Not for me there is no count to give.
const summary = (progress: Progress) =>
  progress.total === 0 ? "Nothing left on your list" : progressLabel(progress);

const ProgressRing = ({ done, total }: { done: number; total: number }) => {
  const radius = 7;
  const circumference = 2 * Math.PI * radius;
  const fraction = total === 0 ? 1 : done / total;
  return (
    <svg
      aria-hidden="true"
      className="h-[18px] w-[18px] shrink-0 -rotate-90"
      viewBox="0 0 18 18"
    >
      <circle
        className="stroke-muted-foreground/25"
        cx="9"
        cy="9"
        fill="none"
        r={radius}
        strokeWidth="2.5"
      />
      <circle
        className="stroke-primary transition-[stroke-dashoffset] duration-500"
        cx="9"
        cy="9"
        fill="none"
        r={radius}
        strokeDasharray={circumference}
        strokeDashoffset={circumference * (1 - fraction)}
        strokeLinecap="round"
        strokeWidth="2.5"
      />
    </svg>
  );
};

interface PillProps extends ButtonProps {
  progress: Progress;
}

// Forwards its ref and props so PopoverTrigger can anchor to it.
const Pill = forwardRef<HTMLButtonElement, PillProps>(
  ({ progress, ...props }, ref) => (
    <Button
      aria-label={`Getting Started, ${progress.complete ? "all set" : progressLabel(progress)}`}
      className="gap-2 rounded-full px-2.5 sm:px-3"
      ref={ref}
      variant="outline"
      {...props}
    >
      <ProgressRing done={progress.done} total={progress.total} />
      <span className="hidden text-sm font-medium sm:inline">
        Getting Started
      </span>
    </Button>
  ),
);
Pill.displayName = "GettingStartedPill";

interface StepRowProps {
  step: GettingStartedStep;
  onNavigate: () => void;
}

const StepRow = ({ step, onNavigate }: StepRowProps) => {
  const copy = STEP_COPY[step.key];
  const setNotForMe = useSetGettingStartedNotForMe();
  const skipped = step.not_for_me && !step.done;

  const toggleNotForMe = async () => {
    try {
      await setNotForMe.mutateAsync({ step: step.key, notForMe: !skipped });
    } catch (error) {
      toastRequestError(error, "Failed to update Getting Started");
    }
  };

  return (
    <li className="flex items-start gap-3 py-3">
      {step.done ? (
        <CircleCheck
          aria-hidden="true"
          className="mt-0.5 h-5 w-5 shrink-0 text-primary"
        />
      ) : (
        <CircleDashed
          aria-hidden="true"
          className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground"
        />
      )}
      <div className="min-w-0 flex-1">
        {step.done || skipped ? (
          <p
            className={cn(
              "text-sm font-medium text-muted-foreground",
              step.done && "line-through",
            )}
          >
            {copy.title}
          </p>
        ) : (
          <Link
            className="text-sm font-medium hover:underline"
            onClick={onNavigate}
            to={copy.to}
          >
            {copy.title}
          </Link>
        )}
        <p className="text-xs text-muted-foreground">
          {step.done
            ? "Done"
            : skipped
              ? "Marked Not for me"
              : copy.description}
        </p>
      </div>
      {!step.done && (
        <Button
          aria-label={
            skipped
              ? `Undo Not for me: ${copy.title}`
              : `Not for me: ${copy.title}`
          }
          // Ghost buttons pull their padding into the margin so their text
          // lines up with the progress count above.
          className="-mr-3 shrink-0 text-muted-foreground"
          disabled={setNotForMe.isPending}
          onClick={toggleNotForMe}
          size="sm"
          variant="ghost"
        >
          {skipped ? "Undo" : "Not for me"}
        </Button>
      )}
    </li>
  );
};

interface PanelBodyProps {
  progress: Progress;
  titleId: string;
  titleRef: RefObject<HTMLHeadingElement | null>;
  /** Whether the title is rendered by the sheet's own header. */
  showTitle: boolean;
  onNavigate: () => void;
  onDismiss: () => void;
  dismissing: boolean;
}

const PanelBody = ({
  progress,
  titleId,
  titleRef,
  showTitle,
  onNavigate,
  onDismiss,
  dismissing,
}: PanelBodyProps) => {
  return (
    <div>
      {showTitle && (
        <div className="flex items-baseline justify-between gap-2">
          <h2
            className="text-base font-semibold focus:outline-none"
            id={titleId}
            ref={titleRef}
            tabIndex={-1}
          >
            Getting Started
          </h2>
          <p className="text-xs text-muted-foreground">{summary(progress)}</p>
        </div>
      )}
      {/* The steps stay below, so a step marked Not for me by mistake can
          still be undone. */}
      {progress.complete && (
        <div className="mt-3 flex items-center gap-3 rounded-md bg-primary/10 p-3">
          <PartyPopper
            aria-hidden="true"
            className="h-6 w-6 shrink-0 text-primary"
          />
          <div>
            <p className="text-sm font-semibold">You're all set</p>
            <p className="text-xs text-muted-foreground">
              {progress.total === 0
                ? "Every step is marked Not for me."
                : "You've finished your first steps."}
            </p>
          </div>
        </div>
      )}
      <ul className="divide-y divide-border">
        {progress.steps.map((step) => (
          <StepRow key={step.key} onNavigate={onNavigate} step={step} />
        ))}
      </ul>
      <div className="flex justify-end border-t border-border pt-3">
        {progress.complete ? (
          <Button disabled={dismissing} onClick={onDismiss} size="sm">
            Done
          </Button>
        ) : (
          <Button
            className="-mr-3 text-muted-foreground"
            disabled={dismissing}
            onClick={onDismiss}
            size="sm"
            variant="ghost"
          >
            Dismiss this list
          </Button>
        )}
      </div>
    </div>
  );
};

/**
 * The "Getting Started" pill in the top nav and the panel it opens: a
 * popover on wider screens, a bottom sheet on phones. It shows while the
 * user's list is on, not dismissed, and has a step for their role. Once
 * every step is done or Not for me, the panel says "You're all set" and
 * closing it hides the pill until the user menu restores it or a step that
 * didn't apply before does.
 */
const GettingStarted = () => {
  const query = useGettingStarted();
  const { data, refetch } = query;
  const { isOpen, setOpen } = useGettingStartedPanel();
  const isPhone = useIsPhone();
  const titleId = useId();
  const titleRef = useRef<HTMLHeadingElement>(null);
  const dismiss = useDismissGettingStarted();
  const clearAutoOpen = useClearGettingStartedAutoOpen();
  const autoOpened = useRef(false);

  const progress =
    data?.enabled && !data.dismissed ? progressOf(data) : undefined;
  const visible = Boolean(progress && progress.steps.length > 0);

  // The panel opens on its own once, on the first visit. Clearing the flag
  // clears it in the cache at once, so a page that mounts its own nav before
  // the request returns doesn't open it again.
  const autoOpen = visible && data?.auto_open;
  useEffect(() => {
    if (!autoOpen || autoOpened.current) return;
    autoOpened.current = true;
    setOpen(true);
    clearAutoOpen.mutate(undefined, {
      onError: (error) =>
        toastRequestError(error, "Failed to update Getting Started"),
    });
  }, [autoOpen, clearAutoOpen, setOpen]);

  // The panel closes even when the dismissal fails, so a failing request
  // can't trap the user in the modal sheet.
  const handleDismiss = async () => {
    try {
      await dismiss.mutateAsync();
      // Closing "You're all set" hides the pill too, so either way the user
      // hears where the list went.
      toast.success("Getting Started is hidden", {
        description: "Bring it back from your account menu.",
        id: GETTING_STARTED_HIDDEN_TOAST_ID,
      });
    } catch (error) {
      toastRequestError(error, "Failed to dismiss Getting Started");
    } finally {
      setOpen(false);
    }
  };

  const handleOpenChange = (open: boolean) => {
    if (open) {
      // Steps tick off as their data appears anywhere, so the list is
      // refreshed whenever it is opened.
      void refetch();
      setOpen(true);
      return;
    }
    // "You're all set" shows once; closing it hides the pill.
    if (progress?.complete) {
      void handleDismiss();
      return;
    }
    setOpen(false);
  };

  // A refetch that hides the list (dismissed in another tab) closes it.
  useEffect(() => {
    if (!visible && isOpen) setOpen(false);
  }, [visible, isOpen, setOpen]);

  if (!visible || !progress) return null;

  const close = () => setOpen(false);
  // Opening focuses the title rather than the first control, so a stray
  // Enter can't mark the first step Not for me.
  const focusTitle = (e: Event) => {
    e.preventDefault();
    titleRef.current?.focus();
  };
  const body = (showTitle: boolean) => (
    <PanelBody
      dismissing={dismiss.isPending}
      onDismiss={handleDismiss}
      onNavigate={close}
      progress={progress}
      showTitle={showTitle}
      titleId={titleId}
      titleRef={titleRef}
    />
  );

  if (isPhone) {
    return (
      <Sheet onOpenChange={handleOpenChange} open={isOpen}>
        <SheetTrigger asChild>
          <Pill progress={progress} />
        </SheetTrigger>
        <SheetContent
          className="flex max-h-[85vh] flex-col rounded-t-md px-4 pb-6"
          data-layout="sheet"
          onOpenAutoFocus={focusTitle}
          side="bottom"
        >
          <SheetHeader className="text-left">
            <SheetTitle
              className="focus:outline-none"
              ref={titleRef}
              tabIndex={-1}
            >
              Getting Started
            </SheetTitle>
            <SheetDescription>{summary(progress)}</SheetDescription>
          </SheetHeader>
          {/* Only the steps scroll, so the title and close button stay put. The
              scroller spans the sheet's padding so the buttons that pull into
              it aren't clipped. */}
          <div className="-mx-4 min-h-0 overflow-y-auto px-4">
            {body(false)}
          </div>
        </SheetContent>
      </Sheet>
    );
  }

  return (
    <Popover onOpenChange={handleOpenChange} open={isOpen}>
      <PopoverTrigger asChild>
        <Pill progress={progress} />
      </PopoverTrigger>
      <PopoverContent
        align="end"
        aria-labelledby={titleId}
        className="w-96 max-w-[calc(100vw-2rem)]"
        data-layout="popover"
        onOpenAutoFocus={focusTitle}
      >
        {body(true)}
      </PopoverContent>
    </Popover>
  );
};

export default GettingStarted;
