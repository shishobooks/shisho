import { useEffect, useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import {
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { useCreateDevice } from "@/hooks/queries/devices";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { toastRequestError } from "@/libraries/api";
import {
  DeviceTypeKobo,
  type DeviceResponse,
  type DeviceType,
} from "@/types/generated/devices";
import { DEVICE_TYPE_LABELS, DEVICE_TYPE_OPTIONS } from "@/utils/devices";

interface CreateDeviceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (device: DeviceResponse) => void;
}

const CreateDeviceDialog = ({
  open,
  onOpenChange,
  onCreated,
}: CreateDeviceDialogProps) => {
  const [name, setName] = useState("");
  const [deviceType, setDeviceType] = useState<DeviceType>(DeviceTypeKobo);
  const createDevice = useCreateDevice();

  useEffect(() => {
    if (open) {
      setName("");
      setDeviceType(DeviceTypeKobo);
    }
  }, [open]);

  const hasChanges = name.trim() !== "" || deviceType !== DeviceTypeKobo;
  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      const device = await createDevice.mutateAsync({
        name: name.trim(),
        device_type: deviceType,
      });
      requestClose();
      onCreated(device);
    } catch (error) {
      toastRequestError(error, "Failed to add device");
    }
  };

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Add Device</DialogTitle>
            <DialogDescription>
              Add a Device for each reading app you use, so you can tell them
              apart and revoke one on its own.
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-6">
            <div className="space-y-2">
              <Label htmlFor="device-name">Name</Label>
              <Input
                id="device-name"
                maxLength={100}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g., Kobo Libra 2"
                value={name}
              />
            </div>
            <div className="space-y-2">
              <Label id="device-type-label">Type</Label>
              <RadioGroup
                aria-labelledby="device-type-label"
                className="space-y-2"
                name="device-type"
                onValueChange={(value) => setDeviceType(value as DeviceType)}
                value={deviceType}
              >
                {DEVICE_TYPE_OPTIONS.map((option) => (
                  <div className="flex items-start gap-3" key={option.type}>
                    <RadioGroupItem
                      className="mt-0.5"
                      id={`device-type-${option.type}`}
                      value={option.type}
                    />
                    <Label
                      className="cursor-pointer font-normal leading-snug"
                      htmlFor={`device-type-${option.type}`}
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
          </DialogBody>
          <DialogFooter>
            <Button
              disabled={createDevice.isPending || !name.trim()}
              size="sm"
              type="submit"
            >
              {createDevice.isPending ? "Adding..." : "Add Device"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </FormDialog>
  );
};

export default CreateDeviceDialog;
