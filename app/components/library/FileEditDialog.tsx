import { Loader2, Plus, Upload, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useUpdateFile, useUploadFileCover } from "@/hooks/queries/books";
import type { File } from "@/types";

interface FileEditDialogProps {
  file: File;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function FileEditDialog({
  file,
  open,
  onOpenChange,
}: FileEditDialogProps) {
  const [narrators, setNarrators] = useState<string[]>(
    file.narrators?.map((n) => n.person?.name || "") || [],
  );
  const [newNarrator, setNewNarrator] = useState("");
  const [coverCacheBuster, setCoverCacheBuster] = useState(Date.now());
  const fileInputRef = useRef<HTMLInputElement>(null);

  const updateFileMutation = useUpdateFile();
  const uploadCoverMutation = useUploadFileCover();

  // Reset form when dialog opens with new file data
  useEffect(() => {
    if (open) {
      setNarrators(file.narrators?.map((n) => n.person?.name || "") || []);
    }
  }, [open, file]);

  const handleAddNarrator = () => {
    if (newNarrator.trim() && !narrators.includes(newNarrator.trim())) {
      setNarrators([...narrators, newNarrator.trim()]);
      setNewNarrator("");
    }
  };

  const handleRemoveNarrator = (index: number) => {
    setNarrators(narrators.filter((_, i) => i !== index));
  };

  const handleCoverUpload = async (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const uploadedFile = event.target.files?.[0];
    if (!uploadedFile) return;

    await uploadCoverMutation.mutateAsync({
      id: file.id,
      file: uploadedFile,
    });

    // Update cache buster to force image refresh
    setCoverCacheBuster(Date.now());

    // Reset the file input
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  };

  const handleSubmit = async () => {
    // Check if narrators changed
    const originalNarrators =
      file.narrators?.map((n) => n.person?.name || "") || [];
    if (JSON.stringify(narrators) !== JSON.stringify(originalNarrators)) {
      await updateFileMutation.mutateAsync({
        id: file.id,
        payload: { narrators },
      });
    }

    onOpenChange(false);
  };

  const isLoading =
    updateFileMutation.isPending || uploadCoverMutation.isPending;

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Edit File</DialogTitle>
        </DialogHeader>

        <div className="space-y-6 py-4">
          {/* File Info */}
          <div className="space-y-2">
            <Label>File</Label>
            <div className="flex items-center gap-2">
              <Badge className="uppercase text-xs" variant="secondary">
                {file.file_type}
              </Badge>
              <span className="text-sm text-muted-foreground truncate">
                {file.filepath.split("/").pop()}
              </span>
            </div>
          </div>

          {/* Cover Upload */}
          <div className="space-y-2">
            <Label>Cover Image</Label>
            <div className="w-32 relative group">
              {file.cover_mime_type ? (
                <img
                  alt="File cover"
                  className="w-full h-auto rounded border border-border"
                  src={`/api/books/files/${file.id}/cover?t=${coverCacheBuster}`}
                />
              ) : (
                <div className="w-full aspect-square rounded border border-dashed border-border flex items-center justify-center text-muted-foreground text-xs bg-muted/30">
                  No cover
                </div>
              )}
              {/* Cover upload overlay */}
              <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity rounded flex items-center justify-center">
                <input
                  accept="image/jpeg,image/png,image/webp"
                  className="hidden"
                  onChange={handleCoverUpload}
                  ref={fileInputRef}
                  type="file"
                />
                <Button
                  disabled={uploadCoverMutation.isPending}
                  onClick={() => fileInputRef.current?.click()}
                  size="sm"
                  variant="secondary"
                >
                  {uploadCoverMutation.isPending ? (
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  ) : (
                    <Upload className="h-4 w-4 mr-2" />
                  )}
                  {file.cover_mime_type ? "Replace" : "Upload"}
                </Button>
              </div>
            </div>
          </div>

          {/* Narrators (only for M4B files) */}
          {file.file_type === "m4b" && (
            <div className="space-y-2">
              <Label>Narrators</Label>
              <div className="flex flex-wrap gap-2 mb-2">
                {narrators.map((narrator, index) => (
                  <Badge
                    className="flex items-center gap-1"
                    key={index}
                    variant="secondary"
                  >
                    {narrator}
                    <button
                      className="ml-1 cursor-pointer hover:text-destructive"
                      onClick={() => handleRemoveNarrator(index)}
                      type="button"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </Badge>
                ))}
              </div>
              <div className="flex gap-2">
                <Input
                  onChange={(e) => setNewNarrator(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      handleAddNarrator();
                    }
                  }}
                  placeholder="Add narrator..."
                  value={newNarrator}
                />
                <Button
                  onClick={handleAddNarrator}
                  type="button"
                  variant="outline"
                >
                  <Plus className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button disabled={isLoading} onClick={handleSubmit}>
            {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Save Changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
