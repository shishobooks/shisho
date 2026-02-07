import { Menu } from "lucide-react";
import type { SVGProps } from "react";

interface IconMenuProps extends SVGProps<SVGSVGElement> {
  width?: number;
  height?: number;
}

export default function IconMenu({
  width = 30,
  height = 30,
  ...restProps
}: IconMenuProps) {
  return (
    <Menu
      aria-hidden
      size={Math.min(width, height)}
      strokeWidth={2.1}
      {...restProps}
    />
  );
}
