import { extractSubtitleFromTitle } from "@/utils/extractSubtitle";

interface Props {
  title: string;
  onExtract: (title: string, subtitle: string) => void;
}

export function ExtractSubtitleButton({ title, onExtract }: Props) {
  const split = extractSubtitleFromTitle(title);
  if (!split) return null;
  return (
    <div className="flex justify-end">
      <button
        className="text-xs text-muted-foreground hover:text-foreground cursor-pointer"
        onClick={() => onExtract(split.title, split.subtitle)}
        type="button"
      >
        Extract subtitle
      </button>
    </div>
  );
}
