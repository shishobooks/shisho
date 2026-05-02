import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import { AdvancedOrderSection } from "./AdvancedOrderSection";
import { AdvancedRepositoriesSection } from "./AdvancedRepositoriesSection";

export interface AdvancedPluginsDialogProps {
  defaultSection?: "order" | "repositories";
  onOpenChange: (open: boolean) => void;
  open: boolean;
}

export const AdvancedPluginsDialog = ({
  defaultSection = "order",
  onOpenChange,
  open,
}: AdvancedPluginsDialogProps) => {
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="flex max-h-[85vh] max-w-3xl flex-col overflow-hidden">
        <DialogHeader>
          <DialogTitle>Advanced plugin settings</DialogTitle>
        </DialogHeader>
        <DialogBody className="flex flex-1 flex-col overflow-hidden">
          <Tabs
            className="flex flex-col overflow-hidden"
            defaultValue={defaultSection}
          >
            <TabsList className="w-full justify-start">
              <TabsTrigger value="order">Order</TabsTrigger>
              <TabsTrigger value="repositories">Repositories</TabsTrigger>
            </TabsList>
            <div className="overflow-auto">
              <TabsContent className="mt-4" value="order">
                <AdvancedOrderSection />
              </TabsContent>
              <TabsContent className="mt-4" value="repositories">
                <AdvancedRepositoriesSection />
              </TabsContent>
            </div>
          </Tabs>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
};
