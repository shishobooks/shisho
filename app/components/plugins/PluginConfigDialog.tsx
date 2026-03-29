import equal from "fast-deep-equal";
import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  usePluginConfig,
  useSavePluginConfig,
  useSavePluginFieldSettings,
  type ConfigField,
} from "@/hooks/queries/plugins";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { formatMetadataFieldLabel } from "@/utils/format";

interface PluginConfigDialogProps {
  onOpenChange: (open: boolean) => void;
  open: boolean;
  pluginId: string;
  pluginName: string;
  scope: string;
}

const SECRET_MASK = "***";

export const PluginConfigDialog = ({
  onOpenChange,
  open,
  pluginId,
  pluginName,
  scope,
}: PluginConfigDialogProps) => {
  const { data, isLoading, dataUpdatedAt } = usePluginConfig(
    open ? scope : undefined,
    open ? pluginId : undefined,
  );
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

  // Track previous open state to detect open transitions.
  // Start with false so that if dialog starts open, we detect it as "just opened".
  const prevOpenRef = useRef(false);

  // Track whether we've initialized for this dialog session.
  // This allows data to load after open transition (async fetch).
  const initializedRef = useRef(false);
  const lastDataUpdatedAtRef = useRef(0);

  // Initialize form values from fetched data, only when dialog opens
  // This preserves user edits when data is refetched while dialog is open
  useEffect(() => {
    const justOpened = open && !prevOpenRef.current;
    prevOpenRef.current = open;

    // Reset initialization flag when dialog opens
    if (justOpened) {
      initializedRef.current = false;
    }

    // Re-initialize if data has been refetched since we last initialized
    // (handles stale cache after query invalidation on save)
    if (
      initializedRef.current &&
      dataUpdatedAt > lastDataUpdatedAtRef.current
    ) {
      initializedRef.current = false;
    }

    // Only initialize once per dialog session, and only when data is available
    if (!open || !data || initializedRef.current) return;

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

    // Initialize field settings
    const initialFieldSettings = data.fieldSettings ?? {};
    setFieldSettings(initialFieldSettings);

    // Initialize confidence threshold (API returns 0-1, convert to percentage)
    const threshold =
      data.confidence_threshold != null
        ? Math.round(data.confidence_threshold * 100)
        : null;
    setConfidenceThreshold(threshold);

    // Store initial values for comparison
    setInitialValues({
      formValues: { ...initial },
      fieldSettings: { ...initialFieldSettings },
      confidenceThreshold: threshold,
    });
    // dataUpdatedAt ensures we re-initialize when fresh data arrives after
    // query invalidation (e.g., reopening dialog after save).
  }, [open, data, dataUpdatedAt]);

  // Compute hasChanges by comparing current values to initial values
  const hasChanges = useMemo(() => {
    if (!initialValues) return false;
    return (
      !equal(formValues, initialValues.formValues) ||
      !equal(fieldSettings, initialValues.fieldSettings) ||
      confidenceThreshold !== initialValues.confidenceThreshold
    );
  }, [formValues, fieldSettings, confidenceThreshold, initialValues]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleFieldToggle = (field: string, enabled: boolean) => {
    setFieldSettings((prev) => ({ ...prev, [field]: enabled }));
  };

  const handleSave = () => {
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

    // Save field settings if there are declared fields
    if (data.declaredFields && data.declaredFields.length > 0) {
      const changedFields: Record<string, boolean> = {};
      for (const field of data.declaredFields) {
        const original = data.fieldSettings?.[field] ?? true;
        const current = fieldSettings[field] ?? true;
        if (original !== current) {
          changedFields[field] = current;
        }
      }
      if (Object.keys(changedFields).length > 0) {
        saveFieldSettings.mutate(
          { scope, id: pluginId, fields: changedFields },
          {
            onError: (err) => {
              toast.error(`Failed to save field settings: ${err.message}`);
            },
          },
        );
      }
    }

    saveConfig.mutate(
      {
        scope,
        id: pluginId,
        config,
        confidence_threshold:
          confidenceThreshold != null ? confidenceThreshold / 100 : undefined,
        clear_confidence_threshold:
          confidenceThreshold == null ? true : undefined,
      },
      {
        onSuccess: () => {
          toast.success("Plugin configuration saved.");
          // Reset initial values so hasChanges becomes false, then close via effect
          setInitialValues({
            formValues: { ...formValues },
            fieldSettings: { ...fieldSettings },
            confidenceThreshold,
          });
          requestClose();
        },
        onError: (err) => {
          toast.error(`Failed to save configuration: ${err.message}`);
        },
      },
    );
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
          <select
            className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring md:text-sm"
            id={fieldId}
            onChange={(e) => handleChange(e.target.value)}
            value={value}
          >
            <option value="">Select...</option>
            {field.options?.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        ) : field.type === "textarea" ? (
          <Textarea
            id={fieldId}
            onChange={(e) => handleChange(e.target.value)}
            rows={4}
            value={value}
          />
        ) : field.type === "number" ? (
          <Input
            id={fieldId}
            max={field.max ?? undefined}
            min={field.min ?? undefined}
            onChange={(e) => handleChange(e.target.value)}
            type="number"
            value={value}
          />
        ) : (
          <Input
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

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="overflow-x-hidden">
        <DialogHeader className="pr-8">
          <DialogTitle>Configure {pluginName}</DialogTitle>
          <DialogDescription>
            Adjust plugin settings and configure which metadata fields it can
            modify.
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : data && Object.keys(data.schema).length > 0 ? (
          <div className="space-y-4">
            {Object.entries(data.schema).map(([key, field]) =>
              renderField(key, field),
            )}
          </div>
        ) : !data?.declaredFields?.length ? (
          <p className="py-4 text-center text-sm text-muted-foreground">
            This plugin has no configuration options.
          </p>
        ) : null}

        {data?.declaredFields && data.declaredFields.length > 0 && (
          <>
            <div className="border-t pt-4">
              <Label className="text-base">Metadata Fields</Label>
              <p className="mt-1 text-xs text-muted-foreground">
                Choose which fields this plugin can set during enrichment.
              </p>
            </div>
            <div className="space-y-3">
              {data.declaredFields.map((field) => (
                <div className="flex items-center justify-between" key={field}>
                  <span className="text-sm">
                    {formatMetadataFieldLabel(field)}
                  </span>
                  <Switch
                    checked={fieldSettings[field] ?? true}
                    onCheckedChange={(checked) =>
                      handleFieldToggle(field, checked)
                    }
                  />
                </div>
              ))}
            </div>

            {/* Confidence threshold - only for enricher plugins */}
            <div className="space-y-2 border-t pt-4">
              <Label>Auto-identify confidence threshold</Label>
              <p className="text-xs text-muted-foreground">
                During automatic scans, results with confidence below this
                threshold will be skipped. Leave empty to use the global
                default.
              </p>
              <div className="flex items-center gap-2">
                <Input
                  className="w-24"
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

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={
              isLoading ||
              saveConfig.isPending ||
              saveFieldSettings.isPending ||
              !data ||
              (Object.keys(data.schema).length === 0 &&
                (!data.declaredFields || data.declaredFields.length === 0))
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
        </DialogFooter>
      </DialogContent>
    </FormDialog>
  );
};
