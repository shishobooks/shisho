import { useEffect, useMemo, useState } from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DataSourceManual, type DataSource } from "@/types";
import { forPerson, forTitle } from "@/utils/sortname";

interface SortNameInputProps {
  /** The name/title being edited (for live preview) */
  nameValue: string;
  /** Current sort name/title value */
  sortValue: string;
  /** Source of the current value ("manual" or other) */
  source: DataSource;
  /** Which algorithm to use */
  type: "title" | "person";
  /** Called with empty string (auto) or actual value (manual) */
  onChange: (value: string) => void;
}

export function SortNameInput({
  nameValue,
  sortValue,
  source,
  type,
  onChange,
}: SortNameInputProps) {
  // Checkbox starts checked if source is not manual
  const [isAuto, setIsAuto] = useState(source !== DataSourceManual);
  // Track the manual value separately
  const [manualValue, setManualValue] = useState(sortValue);

  // Compute the auto-generated value
  const generatedValue = useMemo(() => {
    return type === "title" ? forTitle(nameValue) : forPerson(nameValue);
  }, [nameValue, type]);

  // The displayed value depends on mode
  const displayValue = isAuto ? generatedValue : manualValue;

  // Update manual value when sortValue prop changes (dialog reopened)
  useEffect(() => {
    setManualValue(sortValue);
    setIsAuto(source !== DataSourceManual);
  }, [sortValue, source]);

  const handleCheckboxChange = (checked: boolean) => {
    setIsAuto(checked);
    if (checked) {
      // Switching to auto mode - send empty string
      onChange("");
    } else {
      // Switching to manual mode - pre-fill with generated value
      setManualValue(generatedValue);
      onChange(generatedValue);
    }
  };

  const handleInputChange = (value: string) => {
    setManualValue(value);
    onChange(value);
  };

  const label =
    type === "title" ? "Autogenerate sort title" : "Autogenerate sort name";

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <Checkbox
          checked={isAuto}
          id="autogenerate-sort"
          onCheckedChange={handleCheckboxChange}
        />
        <Label className="font-normal" htmlFor="autogenerate-sort">
          {label}
        </Label>
      </div>
      <Input
        disabled={isAuto}
        onChange={(e) => handleInputChange(e.target.value)}
        value={displayValue}
      />
    </div>
  );
}
