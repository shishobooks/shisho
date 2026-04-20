import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { usePluginManifest } from "@/hooks/queries/plugins";

export interface PluginManifestDialogProps {
  id: string;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  scope: string;
}

export const PluginManifestDialog = ({
  id,
  onOpenChange,
  open,
  scope,
}: PluginManifestDialogProps) => {
  const { data, error, isLoading } = usePluginManifest(scope, id, {
    enabled: open,
  });

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-hidden">
        <DialogHeader>
          <DialogTitle>Plugin manifest</DialogTitle>
        </DialogHeader>
        <div className="max-h-[70vh] overflow-auto rounded-md border bg-muted/30 p-4">
          {isLoading && (
            <div className="text-sm text-muted-foreground">Loading…</div>
          )}
          {error && (
            <div className="text-sm text-destructive">{error.message}</div>
          )}
          {data !== undefined && data !== null && (
            <pre className="text-xs">
              <code>{JSON.stringify(data, null, 2)}</code>
            </pre>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
};
