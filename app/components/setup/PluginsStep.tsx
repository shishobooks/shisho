import { ChevronDown, CloudOff } from "lucide-react";
import { useId, useState } from "react";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { detectionSummary } from "@/components/setup/detectionSummary";
import { PluginChoiceCard } from "@/components/setup/PluginChoiceCard";
import {
  pluginKey,
  type PluginChoices,
} from "@/components/setup/pluginRecommendations";
import { Button } from "@/components/ui/button";
import type { DetectFileTypesResponse } from "@/types";

interface PluginsStepProps {
  /** What's still loading, or null once the step is ready. */
  loading: string | null;
  /** The plugin repository couldn't be reached. */
  unreachable: boolean;
  detected: DetectFileTypesResponse | undefined;
  choices: PluginChoices;
  selected: Set<string>;
  installed: Set<string>;
  onToggle: (key: string, checked: boolean) => void;
  onBack: () => void;
  /** Finishes with no plugins. */
  onSkipStep: () => void;
  onFinish: () => void;
}

export const PluginsStep = ({
  loading,
  unreachable,
  detected,
  choices,
  selected,
  installed,
  onToggle,
  onBack,
  onSkipStep,
  onFinish,
}: PluginsStepProps) => {
  const [showAll, setShowAll] = useState(false);
  const recommendedId = useId();
  const allId = useId();

  // A slow disk or repository never holds the admin here: they can go back
  // or move on without plugins while it loads.
  if (loading) {
    return (
      <div className="space-y-6">
        <div className="flex flex-col items-center gap-3 py-10 text-center">
          <LoadingSpinner />
          <p className="text-sm text-muted-foreground">{loading}</p>
        </div>
        <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
          <Button onClick={onBack} variant="outline">
            Back
          </Button>
          <Button onClick={onSkipStep} variant="ghost">
            Skip this step
          </Button>
        </div>
      </div>
    );
  }

  if (unreachable) {
    return (
      <div className="space-y-6">
        <div className="flex items-start gap-3 rounded-md border border-border bg-muted/40 p-4">
          <CloudOff
            aria-hidden="true"
            className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground"
          />
          <div className="space-y-1">
            <p className="text-sm font-medium">
              We couldn't load plugins from the plugin repository
            </p>
            <p className="text-sm text-muted-foreground">
              This server may be offline, or the repository may be unreachable
              from it. You can add plugins later from Administration &gt;
              Plugins.
            </p>
          </div>
        </div>
        <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
          <Button onClick={onBack} variant="outline">
            Back
          </Button>
          <Button onClick={onSkipStep}>Continue</Button>
        </div>
      </div>
    );
  }

  const card = (plugin: PluginChoices["recommended"][number]) => {
    const key = pluginKey(plugin);
    return (
      <PluginChoiceCard
        checked={selected.has(key)}
        installed={installed.has(key)}
        key={key}
        onCheckedChange={(checked) => onToggle(key, checked)}
        plugin={plugin}
      />
    );
  };

  return (
    <div className="space-y-6">
      <p className="text-sm" role="status">
        {detectionSummary(detected)}
      </p>

      <section
        aria-labelledby={recommendedId}
        className="space-y-3"
        role="group"
      >
        <h2 className="text-sm font-medium" id={recommendedId}>
          Recommended for your library
        </h2>
        {choices.recommended.map(card)}
      </section>

      {choices.others.length > 0 && (
        <div className="space-y-3">
          <Button
            aria-controls={allId}
            aria-expanded={showAll}
            onClick={() => setShowAll((v) => !v)}
            size="sm"
            variant="ghost"
          >
            See all plugins
            <ChevronDown
              aria-hidden="true"
              className={
                showAll
                  ? "rotate-180 transition-transform"
                  : "transition-transform"
              }
            />
          </Button>
          {showAll && (
            <section
              aria-labelledby={`${allId}-heading`}
              className="space-y-3"
              id={allId}
              role="group"
            >
              <h2 className="sr-only" id={`${allId}-heading`}>
                All plugins
              </h2>
              {choices.others.map(card)}
            </section>
          )}
        </div>
      )}

      <div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center sm:justify-between">
        <Button onClick={onBack} variant="outline">
          Back
        </Button>
        <div className="flex flex-col-reverse gap-2 sm:flex-row">
          <Button onClick={onSkipStep} variant="ghost">
            Skip this step
          </Button>
          <Button onClick={onFinish}>Finish setup</Button>
        </div>
      </div>
    </div>
  );
};
