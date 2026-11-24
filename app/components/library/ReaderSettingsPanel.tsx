import { Loader2, Minus, Plus, Upload, X } from "lucide-react";
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
  IMPORTED_FONT_PREFIX,
  NUMERIC_SETTINGS,
  READER_LAYOUTS,
  READER_PAGES,
  READER_THEMES,
  stepSetting,
  type NumericSetting,
  type ReaderSettings,
} from "@/libraries/readerSettings";

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

// A row of buttons that picks one option, as the reader's other settings do.
function OptionGroup<T extends string>({
  label,
  options,
  value,
  onChange,
  disabledReason,
  hint,
}: {
  label: string;
  options: readonly T[];
  value: T;
  onChange: (value: T) => void;
  // Disables the group and says why, next to its label.
  disabledReason?: string;
  // Says what the options do, under them.
  hint?: string;
}) {
  const labelId = useId();
  const hintId = useId();
  return (
    <div>
      <p className="text-sm font-medium" id={labelId}>
        {label}
        {disabledReason && (
          <span className="ml-2 text-xs font-normal text-muted-foreground">
            {disabledReason}
          </span>
        )}
      </p>
      <div
        aria-describedby={hint ? hintId : undefined}
        aria-labelledby={labelId}
        className="flex flex-wrap gap-2 mt-2"
        role="group"
      >
        {options.map((option) => (
          <Button
            aria-pressed={value === option}
            disabled={disabledReason !== undefined}
            key={option}
            onClick={() => onChange(option)}
            size="sm"
            variant={value === option ? "default" : "outline"}
          >
            {capitalize(option)}
          </Button>
        ))}
      </div>
      {hint && (
        <p className="text-xs text-muted-foreground mt-1" id={hintId}>
          {hint}
        </p>
      )}
    </div>
  );
}

// A setting set by number: a slider for big moves and buttons for one step.
// The value and the slider follow a drag, and the book reflows only when it
// ends.
function NumberSetting({
  setting,
  label,
  value,
  format,
  ends,
  onChange,
}: {
  setting: NumericSetting;
  label: string;
  value: number;
  format: (value: number) => string;
  // Words for the slider's two ends, which say what the number means.
  ends?: [string, string];
  onChange: (value: number) => void;
}) {
  const { min, max, step } = NUMERIC_SETTINGS[setting];
  const [draft, setDraft] = useState(value);
  useEffect(() => {
    setDraft(value);
  }, [value]);
  const name = label.toLowerCase();
  const decreaseRef = useRef<HTMLButtonElement>(null);
  const increaseRef = useRef<HTMLButtonElement>(null);
  // A step that reaches the end disables its own button, which would drop
  // keyboard focus out of the popover, so focus moves to the other button.
  const stepTo = (direction: 1 | -1) => {
    const next = stepSetting(setting, value, direction);
    onChange(next);
    if (next === (direction === 1 ? max : min)) {
      (direction === 1 ? decreaseRef : increaseRef).current?.focus();
    }
  };
  return (
    <div>
      <div className="flex items-baseline justify-between">
        <p className="text-sm font-medium">{label}</p>
        {/* Announces a step from the buttons, which leave focus on the
            button rather than the slider. */}
        <span
          aria-live="polite"
          className="text-sm text-muted-foreground tabular-nums"
        >
          {format(draft)}
        </span>
      </div>
      <div className="flex items-center gap-2 mt-1">
        <Button
          aria-label={`Decrease ${name}`}
          disabled={value <= min}
          onClick={() => stepTo(-1)}
          ref={decreaseRef}
          size="icon-sm"
          variant="ghost"
        >
          <Minus className="h-4 w-4" />
        </Button>
        <Slider
          max={max}
          min={min}
          onValueChange={([next]) => setDraft(next)}
          onValueCommit={([next]) => onChange(next)}
          step={step}
          thumbLabel={label}
          thumbValueText={format(draft)}
          value={[draft]}
        />
        <Button
          aria-label={`Increase ${name}`}
          disabled={value >= max}
          onClick={() => stepTo(1)}
          ref={increaseRef}
          size="icon-sm"
          variant="ghost"
        >
          <Plus className="h-4 w-4" />
        </Button>
      </div>
      {ends && (
        <div className="flex justify-between px-9 text-xs text-muted-foreground">
          <span>{ends[0]}</span>
          <span>{ends[1]}</span>
        </div>
      )}
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
      <NumberSetting
        format={(size) => `${size} px`}
        label="Font size"
        onChange={(size) => onChange({ size })}
        setting="size"
        value={settings.size}
      />
      <div>
        <div className="flex items-center justify-between">
          <Label htmlFor="reader-force-size">Force font size</Label>
          <Switch
            aria-describedby="reader-force-size-hint"
            checked={settings.forceFontSize}
            id="reader-force-size"
            onCheckedChange={(forceFontSize) => onChange({ forceFontSize })}
          />
        </div>
        <p
          className="text-xs text-muted-foreground mt-1"
          id="reader-force-size-hint"
        >
          For books that set their own text sizes and ignore the font size.
        </p>
      </div>
      <NumberSetting
        ends={["Compact", "Relaxed"]}
        format={(spacing) => spacing.toFixed(2)}
        label="Line spacing"
        onChange={(lineSpacing) => onChange({ lineSpacing })}
        setting="lineSpacing"
        value={settings.lineSpacing}
      />
      <NumberSetting
        ends={["Narrow", "Wide"]}
        format={(margins) => `${margins}%`}
        label="Margins"
        onChange={(margins) => onChange({ margins })}
        setting="margins"
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
      <OptionGroup
        disabledReason={
          settings.layout === "scrolled" ? "Only for Paginated" : undefined
        }
        hint="Auto shows two pages side by side when the window is wider than it is tall and has room for both."
        label="Pages"
        onChange={(pages) => onChange({ pages })}
        options={READER_PAGES}
        value={settings.pages}
      />
      {children}
    </div>
  );
}
