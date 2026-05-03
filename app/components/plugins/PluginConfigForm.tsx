import equal from "fast-deep-equal";
import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  usePluginConfig,
  useSavePluginConfig,
  useSavePluginFieldSettings,
  type ConfigField,
} from "@/hooks/queries/plugins";
import { formatMetadataFieldLabel } from "@/utils/format";

const SECRET_MASK = "***";

export interface PluginConfigFormProps {
  canWrite: boolean;
  id: string;
  onDirtyChange?: (isDirty: boolean) => void;
  scope: string;
}

export const PluginConfigForm = ({
  canWrite,
  id,
  onDirtyChange,
  scope,
}: PluginConfigFormProps) => {
  const { data, isLoading, dataUpdatedAt } = usePluginConfig(scope, id);
  const saveConfig = useSavePluginConfig();
  const saveFieldSettings = useSavePluginFieldSettings();

  const [formValues, setFormValues] = useState<Record<string, string>>({});
  const [fieldSettings, setFieldSettings] = useState<Record<string, boolean>>(
    {},
  );
  const [confidenceThreshold, setConfidenceThreshold] = useState<number | null>(
    null,
  );

  // Store initial values for change detection
  const [initialValues, setInitialValues] = useState<{
    formValues: Record<string, string>;
    fieldSettings: Record<string, boolean>;
    confidenceThreshold: number | null;
  } | null>(null);

  const initializedRef = useRef(false);
  const lastDataUpdatedAtRef = useRef(0);

  // Initialize form values from fetched data. Re-initialize whenever the
  // server returns fresh data (new dataUpdatedAt) to pick up saved values.
  useEffect(() => {
    if (!data) return;

    if (
      initializedRef.current &&
      dataUpdatedAt <= lastDataUpdatedAtRef.current
    ) {
      return;
    }

    initializedRef.current = true;
    lastDataUpdatedAtRef.current = dataUpdatedAt;

    const initial: Record<string, string> = {};
    for (const [key, field] of Object.entries(data.schema)) {
      const value = data.values[key];
      if (value !== undefined && value !== null) {
        initial[key] = String(value);
      } else if (field.default !== undefined && field.default !== null) {
        initial[key] = String(field.default);
      } else {
        initial[key] = field.type === "boolean" ? "false" : "";
      }
    }
    setFormValues(initial);

    const initialFieldSettings = data.fieldSettings ?? {};
    setFieldSettings(initialFieldSettings);

    // API returns 0-1, convert to percentage for display.
    const threshold =
      data.confidence_threshold != null
        ? Math.round(data.confidence_threshold * 100)
        : null;
    setConfidenceThreshold(threshold);

    setInitialValues({
      formValues: { ...initial },
      fieldSettings: { ...initialFieldSettings },
      confidenceThreshold: threshold,
    });
  }, [data, dataUpdatedAt]);

  // Compute hasChanges by comparing current values to initial values
  const hasChanges = useMemo(() => {
    if (!initialValues) return false;
    return (
      !equal(formValues, initialValues.formValues) ||
      !equal(fieldSettings, initialValues.fieldSettings) ||
      confidenceThreshold !== initialValues.confidenceThreshold
    );
  }, [formValues, fieldSettings, confidenceThreshold, initialValues]);

  // Notify parent of dirty state
  useEffect(() => {
    onDirtyChange?.(hasChanges);
  }, [hasChanges, onDirtyChange]);

  const handleFieldToggle = (field: string, enabled: boolean) => {
    setFieldSettings((prev) => ({ ...prev, [field]: enabled }));
  };

  const handleSave = async () => {
    if (!data) return;

    // Build config payload, excluding secret fields that still show the mask
    const config: Record<string, string> = {};
    for (const [key, field] of Object.entries(data.schema)) {
      const value = formValues[key] ?? "";
      if (field.secret && value === SECRET_MASK) {
        // Don't include masked secret values
        continue;
      }
      config[key] = value;
    }

    // Compute which declared fields actually changed.
    const changedFields: Record<string, boolean> = {};
    if (data.declaredFields && data.declaredFields.length > 0) {
      for (const field of data.declaredFields) {
        const original = data.fieldSettings?.[field] ?? true;
        const current = fieldSettings[field] ?? true;
        if (original !== current) {
          changedFields[field] = current;
        }
      }
    }

    // Await both mutations so initialValues is only reset when both succeed.
    // Otherwise a failed field-settings save would silently clear the dirty
    // state and the user could navigate away thinking everything persisted.
    try {
      const tasks: Promise<unknown>[] = [
        saveConfig.mutateAsync({
          scope,
          id,
          config,
          confidence_threshold:
            confidenceThreshold != null ? confidenceThreshold / 100 : undefined,
          clear_confidence_threshold:
            confidenceThreshold == null ? true : undefined,
        }),
      ];
      if (Object.keys(changedFields).length > 0) {
        tasks.push(
          saveFieldSettings.mutateAsync({ scope, id, fields: changedFields }),
        );
      }
      await Promise.all(tasks);

      toast.success("Plugin configuration saved.");
      setInitialValues({
        formValues: { ...formValues },
        fieldSettings: { ...fieldSettings },
        confidenceThreshold,
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Save failed";
      toast.error(`Failed to save configuration: ${msg}`);
    }
  };

  const renderField = (key: string, field: ConfigField) => {
    const value = formValues[key] ?? "";
    const fieldId = `plugin-config-${key}`;

    const handleChange = (newValue: string) => {
      setFormValues((prev) => ({ ...prev, [key]: newValue }));
    };

    return (
      <div className="space-y-2" key={key}>
        <Label htmlFor={fieldId}>
          {field.label}
          {field.required && <span className="ml-1 text-destructive">*</span>}
        </Label>

        {field.type === "boolean" ? (
          <div className="flex items-center gap-2">
            <Switch
              checked={value === "true"}
              disabled={!canWrite}
              id={fieldId}
              onCheckedChange={(checked) =>
                handleChange(checked ? "true" : "false")
              }
            />
            <span className="text-sm text-muted-foreground">
              {value === "true" ? "Enabled" : "Disabled"}
            </span>
          </div>
        ) : field.type === "select" ? (
          <Select
            disabled={!canWrite}
            onValueChange={handleChange}
            value={value || undefined}
          >
            <SelectTrigger id={fieldId}>
              <SelectValue placeholder="Select..." />
            </SelectTrigger>
            <SelectContent>
              {field.options?.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : field.type === "textarea" ? (
          <Textarea
            disabled={!canWrite}
            id={fieldId}
            onChange={(e) => handleChange(e.target.value)}
            rows={4}
            value={value}
          />
        ) : field.type === "number" ? (
          <Input
            disabled={!canWrite}
            id={fieldId}
            max={field.max ?? undefined}
            min={field.min ?? undefined}
            onChange={(e) => handleChange(e.target.value)}
            type="number"
            value={value}
          />
        ) : (
          <Input
            disabled={!canWrite}
            id={fieldId}
            onChange={(e) => handleChange(e.target.value)}
            type={field.secret ? "password" : "text"}
            value={value}
          />
        )}

        {field.description && (
          <p className="text-xs text-muted-foreground">{field.description}</p>
        )}
      </div>
    );
  };

  const hasSchema = data && Object.keys(data.schema).length > 0;
  const hasDeclaredFields =
    data?.declaredFields && data.declaredFields.length > 0;

  return (
    <section className="space-y-4 rounded-md border border-border p-4 md:p-6">
      <h2 className="text-lg font-semibold">Configuration</h2>

      {isLoading ? (
        <div className="flex items-center justify-center py-8">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      ) : hasSchema ? (
        <div className="space-y-4">
          {Object.entries(data!.schema).map(([key, field]) =>
            renderField(key, field),
          )}
        </div>
      ) : !hasDeclaredFields ? (
        <p className="py-4 text-center text-sm text-muted-foreground">
          This plugin has no configuration options.
        </p>
      ) : null}

      {hasDeclaredFields && (
        <>
          <div>
            <Label className="text-base">Metadata Fields</Label>
            <p className="mt-1 text-xs text-muted-foreground">
              Choose which fields this plugin can set during enrichment.
            </p>
          </div>
          <div className="space-y-3">
            {data!.declaredFields!.map((field) => (
              <div className="flex items-center justify-between" key={field}>
                <span className="text-sm">
                  {formatMetadataFieldLabel(field)}
                </span>
                <Switch
                  checked={fieldSettings[field] ?? true}
                  disabled={!canWrite}
                  onCheckedChange={(checked) =>
                    handleFieldToggle(field, checked)
                  }
                />
              </div>
            ))}
          </div>

          {/* Confidence threshold - only for enricher plugins */}
          <div className="space-y-2">
            <Label>Auto-identify confidence threshold</Label>
            <p className="text-xs text-muted-foreground">
              During automatic scans, results with confidence below this
              threshold will be skipped. Leave empty to use the global default.
            </p>
            <div className="flex items-center gap-2">
              <Input
                className="w-24"
                disabled={!canWrite}
                max={100}
                min={0}
                onChange={(e) => {
                  const val = e.target.value;
                  setConfidenceThreshold(val === "" ? null : Number(val));
                }}
                placeholder="85"
                type="number"
                value={confidenceThreshold ?? ""}
              />
              <span className="text-sm text-muted-foreground">%</span>
            </div>
          </div>
        </>
      )}

      {canWrite && (
        <div className="flex justify-end pt-2">
          <Button
            disabled={
              isLoading ||
              saveConfig.isPending ||
              saveFieldSettings.isPending ||
              !data ||
              (!hasSchema && !hasDeclaredFields)
            }
            onClick={handleSave}
          >
            {saveConfig.isPending || saveFieldSettings.isPending ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Saving...
              </>
            ) : (
              "Save"
            )}
          </Button>
        </div>
      )}
    </section>
  );
};
