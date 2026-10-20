import equal from "fast-deep-equal";
import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import RulesListEditor from "@/components/devices/RulesListEditor";
import { Button } from "@/components/ui/button";
import { useUpdateDevice } from "@/hooks/queries/devices";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import {
  RulesModeCustom,
  RulesModeDefault,
  RulesModePreset,
  type DeliveryRulesMode,
  type DeviceResponse,
} from "@/types/generated/devices";
import { withKeys, withoutKeys, type DraftRule } from "@/utils/delivery";
import { DEVICE_TYPE_LABELS } from "@/utils/devices";
import { fileTypeLabel } from "@/utils/format";

interface DeviceRulesSectionProps {
  device: DeviceResponse;
  /** Reports whether the custom list holds unsaved changes. */
  onHasChangesChange?: (hasChanges: boolean) => void;
}

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
  const modes: { mode: DeliveryRulesMode; label: string }[] = [
    { mode: RulesModeDefault, label: "Follow my default" },
  ];
  if (device.has_preset) {
    modes.push({
      mode: RulesModePreset,
      label: `Follow the ${typeLabel} preset`,
    });
  }
  modes.push({ mode: RulesModeCustom, label: "Customize" });
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
        device.delivery_rules.length === 0 ? (
          <p className="text-sm text-destructive">
            Your default has no rule for a File Type a {typeLabel} Device can
            receive, so it receives nothing. Follow the preset or customize its
            rules.
          </p>
        ) : (
          <ol
            aria-label="Rules this Device follows"
            className="list-decimal space-y-1 pl-5 text-sm"
          >
            {device.delivery_rules.map((rule) => (
              <li key={rule.source_file_type}>
                {fileTypeLabel(rule.source_file_type)} as{" "}
                {fileTypeLabel(rule.delivered_file_type)}
              </li>
            ))}
          </ol>
        )
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
