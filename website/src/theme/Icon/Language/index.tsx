import { Languages } from "lucide-react";
import type { SVGProps } from "react";

interface IconLanguageProps extends SVGProps<SVGSVGElement> {
  width?: number;
  height?: number;
}

export default function IconLanguage({
  width = 20,
  height = 20,
  ...props
}: IconLanguageProps) {
  return (
    <Languages
      aria-hidden
      size={Math.min(width, height)}
      strokeWidth={2.1}
      {...props}
    />
  );
}
