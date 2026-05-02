import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { usePluginManifest } from "@/hooks/queries/plugins";

export interface PluginManifestDialogProps {
  id: string;
  name?: string;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  scope: string;
}

export const PluginManifestDialog = ({
  id,
  name,
  onOpenChange,
  open,
  scope,
}: PluginManifestDialogProps) => {
  const { data, error, isLoading } = usePluginManifest(scope, id, {
    enabled: open,
  });

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      {/* `overflow-hidden` on DialogContent (rather than the usual
          `overflow-x-hidden`) is deliberate here: the header must stay
          sticky while the JSON body scrolls in both axes. The inner
          container owns the scroll via its own max-h + overflow-auto. */}
      <DialogContent className="max-w-3xl max-h-[85vh] overflow-hidden">
        <DialogHeader>
          <DialogTitle>
            {name ? `${name} manifest` : "Plugin manifest"}
          </DialogTitle>
        </DialogHeader>
        <DialogBody>
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
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
};
