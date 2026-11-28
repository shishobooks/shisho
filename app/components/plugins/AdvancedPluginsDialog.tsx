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
import { isAdvancedSection, type AdvancedSection } from "./advancedSections";

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
      <DialogContent className="max-h-[85vh] max-w-3xl">
        <DialogHeader>
          <DialogTitle>Advanced plugin settings</DialogTitle>
        </DialogHeader>
        {/* The tabs stay put and only the section below them scrolls. */}
        <DialogBody className="flex flex-col overflow-hidden">
          <Tabs
            className="flex min-h-0 flex-1 flex-col"
            onValueChange={(value) => {
              if (isAdvancedSection(value)) {
                onSectionChange(value);
              }
            }}
            value={section}
          >
            <TabsList className="w-full shrink-0 justify-start">
              <TabsTrigger value="order">Order</TabsTrigger>
              <TabsTrigger value="repositories">Repositories</TabsTrigger>
            </TabsList>
            <div className="min-h-0 flex-1 overflow-y-auto">
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
