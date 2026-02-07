import { TriangleAlert } from "lucide-react";
import type { SVGProps } from "react";

export default function AdmonitionIconWarning(props: SVGProps<SVGSVGElement>) {
  return <TriangleAlert aria-hidden size={18} strokeWidth={2.1} {...props} />;
}
