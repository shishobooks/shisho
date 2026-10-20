import equal from "fast-deep-equal";
import { ArrowRight, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import RulesListEditor from "@/components/devices/RulesListEditor";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useUpdateDevice } from "@/hooks/queries/devices";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type { Rule } from "@/types";
import {
  RulesModeCustom,
  RulesModeDefault,
  RulesModePreset,
  type DeliveryRulesMode,
  type DeviceResponse,
} from "@/types/generated/devices";
import {
  DELIVERY_RULES_SECTION_ID,
  withKeys,
  withoutKeys,
  type DraftRule,
} from "@/utils/delivery";
import { DEVICE_TYPE_LABELS } from "@/utils/devices";
import { fileTypeLabel } from "@/utils/format";

interface DeviceRulesSectionProps {
  device: DeviceResponse;
  /** Reports whether the custom list holds unsaved changes. */
  onHasChangesChange?: (hasChanges: boolean) => void;
}

// ReadOnlyRules shows the rules a Device follows, in order, without editing
// controls.
const ReadOnlyRules = ({ rules }: { rules: Rule[] }) => (
  <ol aria-label="Rules this Device follows" className="space-y-2">
    {rules.map((rule, index) => {
      const source = fileTypeLabel(rule.source_file_type);
      const delivered = fileTypeLabel(rule.delivered_file_type);
      return (
        <li
          aria-label={`${source} as ${delivered}`}
          className="flex items-center gap-3 rounded-md border border-border px-3 py-2"
          key={rule.source_file_type}
        >
          <span className="w-4 text-right text-xs tabular-nums text-muted-foreground">
            {index + 1}
          </span>
          {/* A fixed width lines the arrows up down the list. */}
          <Badge className="w-14 justify-center" variant="secondary">
            {source}
          </Badge>
          <ArrowRight
            aria-hidden="true"
            className="h-4 w-4 text-muted-foreground"
          />
          <Badge variant="outline">{delivered}</Badge>
        </li>
      );
    })}
  </ol>
);

// DeviceRulesBody chooses which Delivery Rules a Device follows: the user's
// default, its type's preset, or a custom list. Following saves as soon as
// it is chosen. Customizing opens an editor on a copy of what the Device
// follows now, saved only with Save.
const DeviceRulesBody = ({
  device,
  onHasChangesChange,
}: DeviceRulesSectionProps) => {
  const updateDevice = useUpdateDevice();
  const savedCustom = device.delivery_rules_mode === RulesModeCustom;
  // The custom list being edited, or null while the Device follows a list
  // and nobody is customizing it.
  const [draft, setDraft] = useState<DraftRule[] | null>(() =>
    savedCustom ? withKeys(device.delivery_rules) : null,
  );

  const hasChanges =
    draft !== null &&
    (!savedCustom || !equal(withoutKeys(draft), device.delivery_rules));
  useEffect(() => {
    onHasChangesChange?.(hasChanges);
  }, [hasChanges, onHasChangesChange]);

  const save = async (mode: DeliveryRulesMode, rules?: DraftRule[]) => {
    try {
      const saved = await updateDevice.mutateAsync({
        id: device.id,
        payload: {
          delivery_rules: { mode, rules: rules && withoutKeys(rules) },
        },
      });
      setDraft(
        saved.delivery_rules_mode === RulesModeCustom
          ? withKeys(saved.delivery_rules)
          : null,
      );
      toast.success("Delivery Rules saved");
    } catch (error) {
      toastRequestError(error, "Failed to save Delivery Rules");
    }
  };

  const typeLabel = DEVICE_TYPE_LABELS[device.device_type];
  // Short labels, so three fit side by side on one line.
  const modes: { mode: DeliveryRulesMode; label: string }[] = [
    { mode: RulesModeDefault, label: "My default" },
  ];
  if (device.has_preset) {
    modes.push({ mode: RulesModePreset, label: `${typeLabel} preset` });
  }
  modes.push({ mode: RulesModeCustom, label: "Custom" });
  const followsDefault =
    draft === null && device.delivery_rules_mode === RulesModeDefault;
  const shownMode =
    draft !== null ? RulesModeCustom : device.delivery_rules_mode;

  const choose = (mode: DeliveryRulesMode) => {
    if (mode === RulesModeCustom) {
      if (draft === null) setDraft(withKeys(device.delivery_rules));
      return;
    }
    if (mode === device.delivery_rules_mode && draft === null) return;
    void save(mode);
  };

  return (
    <>
      <div
        aria-label="Delivery Rules"
        className="flex rounded-md border border-input"
        role="group"
      >
        {modes.map((option, i) => (
          <Button
            aria-pressed={shownMode === option.mode}
            className={cn(
              "flex-1 px-3 py-2 text-sm font-medium transition-colors first:rounded-l-md last:rounded-r-md",
              i > 0 && "border-l border-input",
              shownMode === option.mode
                ? "bg-primary text-primary-foreground"
                : "hover:bg-muted",
            )}
            disabled={updateDevice.isPending}
            key={option.mode}
            onClick={() => choose(option.mode)}
            variant="unstyled"
          >
            {option.label}
          </Button>
        ))}
      </div>

      {draft === null ? (
        <>
          {device.delivery_rules.length === 0 ? (
            <p className="text-sm text-destructive">
              Your default has no rule for a File Type a {typeLabel} Device can
              receive, so it receives nothing. Follow the preset or customize
              its rules.
            </p>
          ) : (
            <ReadOnlyRules rules={device.delivery_rules} />
          )}
          {followsDefault && (
            <p className="text-sm text-muted-foreground">
              Changes to your default reach this Device too.{" "}
              <Link
                className="underline underline-offset-4 hover:text-foreground"
                to={`/user/settings#${DELIVERY_RULES_SECTION_ID}`}
              >
                Edit your default
              </Link>
            </p>
          )}
        </>
      ) : (
        <>
          <RulesListEditor
            onChange={setDraft}
            options={device.delivery_rule_options}
            rules={draft}
          />
          <div className="flex flex-wrap items-center gap-2">
            <Button
              disabled={
                !hasChanges || draft.length === 0 || updateDevice.isPending
              }
              onClick={() => save(RulesModeCustom, draft)}
              size="sm"
            >
              {updateDevice.isPending && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Save
            </Button>
            {hasChanges && (
              <Button
                disabled={updateDevice.isPending}
                onClick={() =>
                  setDraft(savedCustom ? withKeys(device.delivery_rules) : null)
                }
                size="sm"
                variant="ghost"
              >
                {savedCustom ? "Discard changes" : "Cancel"}
              </Button>
            )}
          </div>
        </>
      )}
    </>
  );
};

// DeviceRulesSection is the Device page's Delivery Rules control. It
// remounts when the Device or its type changes, so the editor never holds a
// list for a type that can't receive it.
const DeviceRulesSection = (props: DeviceRulesSectionProps) => (
  <DeviceRulesBody
    key={`${props.device.id}-${props.device.device_type}`}
    {...props}
  />
);

export default DeviceRulesSection;
