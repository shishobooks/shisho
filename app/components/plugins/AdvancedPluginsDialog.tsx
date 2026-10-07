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

export type AdvancedSection = "order" | "repositories";

export interface AdvancedPluginsDialogProps {
  onOpenChange: (open: boolean) => void;
  onSectionChange: (section: AdvancedSection) => void;
  open: boolean;
  section: AdvancedSection;
}

export const AdvancedPluginsDialog = ({
  onOpenChange,
  onSectionChange,
  open,
  section,
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
            onValueChange={(value) => {
              if (value === "order" || value === "repositories") {
                onSectionChange(value);
              }
            }}
            value={section}
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
