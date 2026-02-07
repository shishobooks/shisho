import { X } from "lucide-react";
import type { SVGProps } from "react";

interface IconCloseProps extends SVGProps<SVGSVGElement> {
  width?: number;
  height?: number;
}

export default function IconClose({
  width = 21,
  height = 21,
  ...restProps
}: IconCloseProps) {
  return (
    <X
      aria-hidden
      size={Math.min(width, height)}
      strokeWidth={2.1}
      {...restProps}
    />
  );
}
