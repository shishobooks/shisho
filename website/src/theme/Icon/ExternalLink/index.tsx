import { translate } from "@docusaurus/Translate";
import { ExternalLink } from "lucide-react";

interface IconExternalLinkProps {
  width?: number;
  height?: number;
}

export default function IconExternalLink({
  width = 13.5,
  height = 13.5,
}: IconExternalLinkProps) {
  const size = Math.max(width, height);

  return (
    <ExternalLink
      aria-label={translate({
        id: "theme.IconExternalLink.ariaLabel",
        message: "(opens in new tab)",
        description: "The ARIA label for the external link icon",
      })}
      className="icon-external-link"
      size={size}
      strokeWidth={2.1}
    />
  );
}
