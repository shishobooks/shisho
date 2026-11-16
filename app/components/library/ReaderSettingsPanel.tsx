import { Loader2, Upload, X } from "lucide-react";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import {
  BUNDLED_FONTS,
  FontImportError,
  type ImportedFont,
} from "@/libraries/readerFonts";
import {
  BOOK_FONT,
  FONT_SIZE,
  IMPORTED_FONT_PREFIX,
  LINE_SPACINGS,
  READER_LAYOUTS,
  READER_MARGINS,
  READER_THEMES,
  type ReaderSettings,
} from "@/libraries/readerSettings";

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

// A row of buttons that picks one option, as the reader's other settings do.
function OptionGroup<T extends string>({
  label,
  options,
  value,
  onChange,
}: {
  label: string;
  options: readonly T[];
  value: T;
  onChange: (value: T) => void;
}) {
  const labelId = useId();
  return (
    <div>
      <p className="text-sm font-medium" id={labelId}>
        {label}
      </p>
      <div
        aria-labelledby={labelId}
        className="flex flex-wrap gap-2 mt-2"
        role="group"
      >
        {options.map((option) => (
          <Button
            aria-pressed={value === option}
            key={option}
            onClick={() => onChange(option)}
            size="sm"
            variant={value === option ? "default" : "outline"}
          >
            {capitalize(option)}
          </Button>
        ))}
      </div>
    </div>
  );
}

interface ReaderSettingsPanelProps {
  settings: ReaderSettings;
  onChange: (change: Partial<ReaderSettings>) => void;
  importedFonts: ImportedFont[];
  onImportFont: (file: File) => Promise<ImportedFont>;
  onRemoveFont: (id: number) => Promise<void>;
  // Settings that aren't display settings, shown after them.
  children?: ReactNode;
}

// The ebook reader's display settings. Each change applies at once.
export default function ReaderSettingsPanel({
  settings,
  onChange,
  importedFonts,
  onImportFont,
  onRemoveFont,
  children,
}: ReaderSettingsPanelProps) {
  // The slider's thumb and label follow a drag, and the book reflows only
  // when it ends.
  const [sizeDraft, setSizeDraft] = useState(settings.size);
  useEffect(() => {
    setSizeDraft(settings.size);
  }, [settings.size]);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const [importing, setImporting] = useState(false);

  const fontValue = (font: ImportedFont) => IMPORTED_FONT_PREFIX + font.id;
  // An imported font that was removed in another tab shows as the book's.
  const chosenFont =
    settings.font.startsWith(IMPORTED_FONT_PREFIX) &&
    !importedFonts.some((font) => fontValue(font) === settings.font)
      ? BOOK_FONT
      : settings.font;

  const handleImport = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    setImporting(true);
    try {
      const font = await onImportFont(file);
      onChange({ font: fontValue(font) });
    } catch (err) {
      // Nothing here reaches the server. The reader's own refusals say what
      // was wrong; a storage failure gets a plain message.
      toast.error(
        err instanceof FontImportError
          ? err.message
          : "Failed to import the font",
      );
    } finally {
      setImporting(false);
    }
  };

  const handleRemove = async (font: ImportedFont) => {
    try {
      await onRemoveFont(font.id);
      if (settings.font === fontValue(font)) onChange({ font: BOOK_FONT });
    } catch {
      toast.error(`Failed to remove ${font.name}`);
    }
  };

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="reader-font">Font</Label>
        <Select onValueChange={(font) => onChange({ font })} value={chosenFont}>
          <SelectTrigger className="mt-2" id="reader-font">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={BOOK_FONT}>Book's font</SelectItem>
            <SelectSeparator />
            <SelectGroup>
              <SelectLabel>Included</SelectLabel>
              {BUNDLED_FONTS.map((font) => (
                <SelectItem key={font.id} value={font.id}>
                  {font.name}
                </SelectItem>
              ))}
            </SelectGroup>
            {importedFonts.length > 0 && (
              <>
                <SelectSeparator />
                <SelectGroup>
                  <SelectLabel>Imported</SelectLabel>
                  {importedFonts.map((font) => (
                    <SelectItem key={font.id} value={fontValue(font)}>
                      {font.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </>
            )}
          </SelectContent>
        </Select>
        {importedFonts.length > 0 && (
          <ul aria-label="Imported fonts" className="mt-2 space-y-1">
            {importedFonts.map((font) => (
              <li
                className="flex items-center justify-between gap-2 text-sm"
                key={font.id}
              >
                <span className="min-w-0 truncate" title={font.name}>
                  {font.name}
                </span>
                <Button
                  aria-label={`Remove ${font.name}`}
                  onClick={() => handleRemove(font)}
                  size="icon-sm"
                  variant="ghost"
                >
                  <X className="h-4 w-4" />
                </Button>
              </li>
            ))}
          </ul>
        )}
        <input
          accept=".ttf,.otf,.woff2"
          aria-label="Font file"
          className="hidden"
          onChange={handleImport}
          ref={fileInputRef}
          type="file"
        />
        <Button
          className="mt-2"
          disabled={importing}
          onClick={() => fileInputRef.current?.click()}
          size="sm"
          variant="outline"
        >
          {importing ? (
            <Loader2 className="h-4 w-4 mr-2 animate-spin" />
          ) : (
            <Upload className="h-4 w-4 mr-2" />
          )}
          Import font
        </Button>
        <p className="text-xs text-muted-foreground mt-1">
          TTF, OTF, or WOFF2. Kept in this browser only.
        </p>
      </div>
      <div>
        <p className="text-sm font-medium">Font size: {sizeDraft}%</p>
        <Slider
          className="mt-2"
          max={FONT_SIZE.max}
          min={FONT_SIZE.min}
          onValueChange={([value]) => setSizeDraft(value)}
          onValueCommit={([value]) => onChange({ size: value })}
          step={FONT_SIZE.step}
          thumbLabel="Font size"
          value={[sizeDraft]}
        />
      </div>
      <OptionGroup
        label="Line spacing"
        onChange={(lineSpacing) => onChange({ lineSpacing })}
        options={LINE_SPACINGS}
        value={settings.lineSpacing}
      />
      <OptionGroup
        label="Margins"
        onChange={(margins) => onChange({ margins })}
        options={READER_MARGINS}
        value={settings.margins}
      />
      <div className="flex items-center justify-between">
        <Label htmlFor="reader-justify">Justify text</Label>
        <Switch
          checked={settings.justify}
          id="reader-justify"
          onCheckedChange={(justify) => onChange({ justify })}
        />
      </div>
      <OptionGroup
        label="Theme"
        onChange={(theme) => onChange({ theme })}
        options={READER_THEMES}
        value={settings.theme}
      />
      <OptionGroup
        label="Layout"
        onChange={(layout) => onChange({ layout })}
        options={READER_LAYOUTS}
        value={settings.layout}
      />
      {children}
    </div>
  );
}
