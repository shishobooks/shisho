import { useEffect, useId, useMemo, useState } from "react";

import { Badge, BadgeRemoveButton } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { validateIdentifier } from "@/utils/identifiers";

export interface IdentifierRow {
  type: string;
  value: string;
}

export interface IdentifierTypeOption {
  /** The identifier type id (e.g. "isbn_13"). Matches the existing
   * FileEditDialog usage where the type-list field is `id` (not `type`). */
  id: string;
  label: string;
  /** Optional regex pattern for plugin-defined types, passed through to
   * `validateIdentifier`. */
  pattern?: string;
}

export type IdentifierStatus = "new" | "changed" | "unchanged";

interface IdentifierEditorProps {
  value: IdentifierRow[];
  onChange: (next: IdentifierRow[]) => void;
  identifierTypes: IdentifierTypeOption[];
  hideHeader?: boolean;
}

export function IdentifierEditor({
  value,
  onChange,
  identifierTypes,
  hideHeader,
}: IdentifierEditorProps) {
  const presentTypes = useMemo(
    () => new Set(value.map((row) => row.type)),
    [value],
  );

  const firstAvailable = useMemo(
    () => identifierTypes.find((t) => !presentTypes.has(t.id))?.id ?? "",
    [identifierTypes, presentTypes],
  );

  const [newType, setNewType] = useState<string>(
    firstAvailable || identifierTypes[0]?.id || "",
  );
  const [newValue, setNewValue] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const validationErrorId = useId();

  // Auto-switch the selected identifier type away from one that is already
  // present. Mirrors FileEditDialog's behavior so the dropdown never shows a
  // disabled type as the selected value.
  useEffect(() => {
    if (!presentTypes.has(newType)) return;
    const next = identifierTypes.find((t) => !presentTypes.has(t.id));
    if (next) {
      setNewType(next.id);
    }
  }, [presentTypes, newType, identifierTypes]);

  const labelFor = (typeId: string): string =>
    identifierTypes.find((t) => t.id === typeId)?.label ?? typeId;

  const allTypesPresent = presentTypes.size >= identifierTypes.length;
  const addDisabled = allTypesPresent || !newType || !newValue.trim();

  const handleAdd = () => {
    if (presentTypes.has(newType)) return;
    const trimmed = newValue.trim();
    if (!trimmed) return;

    const option = identifierTypes.find((t) => t.id === newType);
    const validation = validateIdentifier(newType, trimmed, option?.pattern);
    if (!validation.valid) {
      setValidationError(validation.error ?? "Invalid value");
      return;
    }

    onChange([...value, { type: newType, value: trimmed }]);
    setNewValue("");
    setValidationError(null);
  };

  const handleRemove = (idx: number) => {
    onChange(value.filter((_, i) => i !== idx));
  };

  const handleClearAll = () => {
    onChange([]);
  };

  return (
    <div className="space-y-2">
      {!hideHeader && (
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium leading-none">Identifiers</span>
          {value.length > 1 && (
            <Button
              className="text-xs font-normal text-muted-foreground hover:text-destructive hover:no-underline"
              onClick={handleClearAll}
              variant="link"
            >
              Clear all
            </Button>
          )}
        </div>
      )}
      {value.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          {value.map((row, idx) => {
            const label = labelFor(row.type);
            return (
              <Badge
                className="flex items-center gap-1 max-w-full"
                data-testid="identifier-row"
                key={`${row.type}-${idx}`}
                variant="secondary"
              >
                <span className="text-xs">{label}</span>:{" "}
                <span>{row.value}</span>
                <BadgeRemoveButton
                  aria-label={`Remove ${label}`}
                  onClick={() => handleRemove(idx)}
                />
              </Badge>
            );
          })}
          {hideHeader && value.length > 1 && (
            <Button
              className="text-xs font-normal text-muted-foreground hover:text-destructive hover:no-underline"
              onClick={handleClearAll}
              variant="link"
            >
              Clear all
            </Button>
          )}
        </div>
      )}
      <div className="flex gap-2">
        <Select onValueChange={setNewType} value={newType}>
          <SelectTrigger
            aria-label="Identifier type"
            className="w-auto min-w-32 shrink-0 gap-2"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {identifierTypes.map(({ id, label }) => {
              const isPresent = presentTypes.has(id);
              if (!isPresent) {
                return (
                  <SelectItem key={id} value={id}>
                    {label}
                  </SelectItem>
                );
              }
              // Arrow keys skip a disabled option and Tab never enters the
              // list, so the tooltip is pointer-only; the sr-only text tells
              // a screen reader why the option is disabled.
              const item = (
                <SelectItem disabled key={id} value={id}>
                  {label} <span className="sr-only">(already added)</span>
                </SelectItem>
              );
              return (
                <Tooltip key={id}>
                  <TooltipTrigger asChild>
                    <span className="block">{item}</span>
                  </TooltipTrigger>
                  <TooltipContent>
                    {label} is already added. Remove it first to add a different
                    value.
                  </TooltipContent>
                </Tooltip>
              );
            })}
          </SelectContent>
        </Select>
        <Input
          aria-describedby={validationError ? validationErrorId : undefined}
          aria-invalid={validationError ? true : undefined}
          aria-label="Identifier value"
          className="flex-1"
          onChange={(e) => {
            setNewValue(e.target.value);
            if (validationError) setValidationError(null);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              handleAdd();
            }
          }}
          placeholder="Enter value..."
          value={newValue}
        />
        <Button
          disabled={addDisabled}
          onClick={handleAdd}
          type="button"
          variant="outline"
        >
          Add
        </Button>
      </div>
      {validationError && (
        <p
          className="text-xs text-destructive"
          id={validationErrorId}
          role="alert"
        >
          {validationError}
        </p>
      )}
    </div>
  );
}
