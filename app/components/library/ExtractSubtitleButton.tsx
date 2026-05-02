import { Scissors } from "lucide-react";

import { Button } from "@/components/ui/button";
import { extractSubtitleFromTitle } from "@/utils/extractSubtitle";

interface Props {
  title: string;
  onExtract: (title: string, subtitle: string) => void;
}

export function ExtractSubtitleButton({ title, onExtract }: Props) {
  const split = extractSubtitleFromTitle(title);
  if (!split) return null;
  return (
    <Button
      className="h-6 px-2 text-xs"
      onClick={() => onExtract(split.title, split.subtitle)}
      size="sm"
      type="button"
      variant="ghost"
    >
      <Scissors className="h-3 w-3" />
      Extract subtitle
    </Button>
  );
}
