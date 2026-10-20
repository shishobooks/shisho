import { useEffect, useState, type FormEvent } from "react";

import DeviceTypeRadioGroup from "@/components/devices/DeviceTypeRadioGroup";
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
import { useCreateDevice } from "@/hooks/queries/devices";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { toastRequestError } from "@/libraries/api";
import {
  DeviceTypeKobo,
  type DeviceResponse,
  type DeviceType,
} from "@/types/generated/devices";

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
            <DeviceTypeRadioGroup
              id="device-type"
              onChange={setDeviceType}
              value={deviceType}
            />
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
