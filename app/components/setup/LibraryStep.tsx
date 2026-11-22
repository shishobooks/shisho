import { FolderOpen, Loader2, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import DirectoryPickerDialog from "@/components/library/DirectoryPickerDialog";
import { OrganizeAnimation } from "@/components/setup/OrganizeAnimation";
import type {
  LibraryStepErrors,
  SetupDraft,
} from "@/components/setup/setupDraft";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/libraries/utils";

interface LibraryStepProps {
  draft: SetupDraft;
  onChange: (changes: Partial<SetupDraft>) => void;
  /** Errors to show, once the admin has tried to move on. */
  errors: LibraryStepErrors;
  onContinue: () => void;
  continueLabel: string;
  /** The server is looking at the folders before Setup goes on. */
  continuePending?: boolean;
  /** The dev server's sample library, offered as a shortcut in development. */
  devLibraryPath?: string;
}

export const LibraryStep = ({
  draft,
  onChange,
  errors,
  onContinue,
  continueLabel,
  continuePending,
  devLibraryPath,
}: LibraryStepProps) => {
  const [pickerIndex, setPickerIndex] = useState<number | null>(null);
  const { paths } = draft;

  const setPath = (index: number, value: string) =>
    onChange({ paths: paths.map((p, i) => (i === index ? value : p)) });

  // The picker's first choice fills the row it was opened from; any others
  // are added below it.
  const handlePick = (picked: string[]) => {
    if (pickerIndex === null || picked.length === 0) return;
    const next = [...paths];
    next.splice(pickerIndex, 1, ...picked);
    onChange({ paths: next });
    setPickerIndex(null);
  };

  return (
    <form
      className="space-y-8"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        onContinue();
      }}
    >
      <section className="space-y-2">
        <Label htmlFor="setup-library-name">Library name</Label>
        <Input
          aria-describedby={
            errors.name ? "setup-library-name-error" : undefined
          }
          aria-invalid={errors.name ? true : undefined}
          autoComplete="off"
          id="setup-library-name"
          onChange={(e) => onChange({ name: e.target.value })}
          placeholder="Books"
          value={draft.name}
        />
        {errors.name && (
          <p className="text-sm text-destructive" id="setup-library-name-error">
            {errors.name}
          </p>
        )}
      </section>

      <section aria-labelledby="setup-folders-heading" className="space-y-3">
        <div>
          <h2 className="text-sm font-medium" id="setup-folders-heading">
            Folders
          </h2>
          <p className="text-sm text-muted-foreground">
            The folders on your server that hold your ebooks, audiobooks, and
            comics. Shisho looks inside every folder below them.
          </p>
        </div>
        {paths.map((path, index) => {
          const missing =
            path.trim() !== "" &&
            (errors.missingPaths ?? []).includes(path.trim());
          const missingId = `setup-folder-${index}-missing`;
          return (
            <div className="space-y-1" key={index}>
              <div className="flex items-center gap-2">
                <Input
                  aria-describedby={
                    [
                      missing && missingId,
                      errors.paths && "setup-folders-error",
                    ]
                      .filter(Boolean)
                      .join(" ") || undefined
                  }
                  aria-invalid={missing || errors.paths ? true : undefined}
                  aria-label={`Folder ${index + 1}`}
                  className="min-w-0 flex-1 font-mono"
                  onChange={(e) => setPath(index, e.target.value)}
                  placeholder="/media"
                  value={path}
                />
                <Button
                  aria-label={`Browse for folder ${index + 1}`}
                  className="shrink-0"
                  onClick={() => setPickerIndex(index)}
                  variant="outline"
                >
                  <FolderOpen aria-hidden="true" className="h-4 w-4 sm:mr-2" />
                  <span className="hidden sm:inline">Browse</span>
                </Button>
                {paths.length > 1 && (
                  <Button
                    aria-label={`Remove folder ${index + 1}`}
                    className="shrink-0"
                    onClick={() =>
                      onChange({ paths: paths.filter((_, i) => i !== index) })
                    }
                    size="icon"
                    variant="outline"
                  >
                    <Trash2 aria-hidden="true" className="h-4 w-4" />
                  </Button>
                )}
              </div>
              {missing && (
                <p className="text-sm text-destructive" id={missingId}>
                  Shisho can't find or open this folder on the server. Check the
                  path, or choose the folder with Browse.
                </p>
              )}
            </div>
          );
        })}
        {errors.paths && (
          <p className="text-sm text-destructive" id="setup-folders-error">
            {errors.paths}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button
            onClick={() => onChange({ paths: [...paths, ""] })}
            size="sm"
            variant="outline"
          >
            <Plus aria-hidden="true" className="h-4 w-4" />
            Add another folder
          </Button>
          {devLibraryPath && (
            <Button
              onClick={() =>
                onChange({
                  name: draft.name.trim() ? draft.name : "Main",
                  paths: [devLibraryPath],
                })
              }
              size="sm"
              variant="outline"
            >
              Use dev library
            </Button>
          )}
        </div>
      </section>

      <section aria-labelledby="setup-organize-heading" className="space-y-3">
        <div>
          <h2 className="text-sm font-medium" id="setup-organize-heading">
            Organize your files
          </h2>
          <p className="text-sm text-muted-foreground">
            As it scans, Shisho can move and rename your files so each book has
            its own folder, named for its author and title.
          </p>
        </div>
        <OrganizeAnimation />
        <div
          className={cn(
            "flex items-start gap-3 rounded-md border border-border p-3",
            !draft.organize && "bg-muted/40",
          )}
        >
          <Checkbox
            aria-describedby="setup-organize-help"
            checked={draft.organize}
            className="mt-0.5"
            id="setup-organize"
            onCheckedChange={(checked) =>
              onChange({ organize: checked === true })
            }
          />
          <div className="space-y-1">
            <Label className="w-fit cursor-pointer" htmlFor="setup-organize">
              Organize file structure during scans
            </Label>
            <p
              className="text-sm text-muted-foreground"
              id="setup-organize-help"
            >
              {draft.organize
                ? "Recommended. Back up your files first: organizing changes them on disk. Turn this off to leave your folders exactly as they are."
                : "Shisho will leave your folders exactly as they are. You can turn this on later in the library's settings."}
            </p>
          </div>
        </div>
      </section>

      <div className="flex justify-end">
        <Button
          className="w-full sm:w-auto"
          disabled={continuePending}
          type="submit"
        >
          {continuePending ? (
            <>
              <Loader2 aria-hidden="true" className="animate-spin" />
              Checking folders…
            </>
          ) : (
            continueLabel
          )}
        </Button>
      </div>

      <DirectoryPickerDialog
        initialPath={
          pickerIndex !== null && paths[pickerIndex] ? paths[pickerIndex] : "/"
        }
        onOpenChange={(open) => !open && setPickerIndex(null)}
        onSelect={handlePick}
        open={pickerIndex !== null}
      />
    </form>
  );
};
