import clsx from "clsx";
import { Pencil } from "lucide-react";
import type { SVGProps } from "react";

export default function IconEdit({
  className,
  ...restProps
}: SVGProps<SVGSVGElement>) {
  return (
    <Pencil
      aria-hidden
      className={clsx(className)}
      size={18}
      strokeWidth={2.1}
      {...restProps}
    />
  );
}
