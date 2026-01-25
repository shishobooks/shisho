import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  usePluginConfig,
  useSavePluginConfig,
  type ConfigField,
} from "@/hooks/queries/plugins";

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
  const { data, isLoading } = usePluginConfig(
    open ? scope : undefined,
    open ? pluginId : undefined,
  );
  const saveConfig = useSavePluginConfig();
  const [formValues, setFormValues] = useState<Record<string, string>>({});

  // Initialize form values from fetched data
  useEffect(() => {
    if (data) {
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
    }
  }, [data]);

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

    saveConfig.mutate(
      { scope, id: pluginId, config },
      {
        onSuccess: () => {
          toast.success("Plugin configuration saved.");
          onOpenChange(false);
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
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="overflow-x-hidden">
        <DialogHeader className="pr-8">
          <DialogTitle>Configure {pluginName}</DialogTitle>
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
        ) : (
          <p className="py-4 text-center text-sm text-muted-foreground">
            This plugin has no configuration options.
          </p>
        )}

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={
              isLoading ||
              saveConfig.isPending ||
              !data ||
              Object.keys(data.schema).length === 0
            }
            onClick={handleSave}
          >
            {saveConfig.isPending ? (
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
    </Dialog>
  );
};
