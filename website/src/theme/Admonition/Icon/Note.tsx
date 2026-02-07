import { NotebookPen } from "lucide-react";
import type { SVGProps } from "react";

export default function AdmonitionIconNote(props: SVGProps<SVGSVGElement>) {
  return <NotebookPen aria-hidden size={18} strokeWidth={2.1} {...props} />;
}
