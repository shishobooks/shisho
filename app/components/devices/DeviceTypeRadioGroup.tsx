import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { DeviceType } from "@/types/generated/devices";
import { DEVICE_TYPE_LABELS, DEVICE_TYPE_OPTIONS } from "@/utils/devices";

interface DeviceTypeRadioGroupProps {
  /** Prefixes the group's element ids, unique on the page. */
  id: string;
  value: DeviceType;
  onChange: (value: DeviceType) => void;
}

// DeviceTypeRadioGroup picks a Device type, describing what each is for.
const DeviceTypeRadioGroup = ({
  id,
  value,
  onChange,
}: DeviceTypeRadioGroupProps) => (
  <div className="space-y-2">
    <Label id={`${id}-label`}>Type</Label>
    <RadioGroup
      aria-labelledby={`${id}-label`}
      className="space-y-2"
      name={id}
      onValueChange={(next) => onChange(next as DeviceType)}
      value={value}
    >
      {DEVICE_TYPE_OPTIONS.map((option) => (
        <div className="flex items-start gap-3" key={option.type}>
          <RadioGroupItem
            className="mt-0.5"
            id={`${id}-${option.type}`}
            value={option.type}
          />
          <Label
            className="cursor-pointer font-normal leading-snug"
            htmlFor={`${id}-${option.type}`}
          >
            <span className="block font-medium">
              {DEVICE_TYPE_LABELS[option.type]}
            </span>
            <span className="block text-sm text-muted-foreground">
              {option.description}
            </span>
          </Label>
        </div>
      ))}
    </RadioGroup>
  </div>
);

export default DeviceTypeRadioGroup;
