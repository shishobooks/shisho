import Link from "@docusaurus/Link";
import clsx from "clsx";
import { useId, type ReactNode } from "react";

type LogoSize = "sm" | "md" | "lg";

interface ShishoLogoProps {
  asLink?: boolean;
  className?: string;
  size?: LogoSize;
}

// Shelf mark: four book spines on a shelf. Each book is a flat base rect plus
// three subtle overlays (spine highlight, title band, contact shadow) so the
// icon reads with depth at large sizes while degrading to the plain silhouette
// at 16px. Gradient ids are namespaced per instance because the logo renders
// more than once per page.
const BOOKS = [
  { dim: false, h: 28, w: 7, x: 8, y: 12 },
  { dim: true, h: 32, w: 6, x: 17, y: 8 },
  { dim: false, h: 24, w: 8, x: 25, y: 16 },
  { dim: true, h: 30, w: 5, x: 35, y: 10 },
];

function ShelfIcon({ className }: { className?: string }): ReactNode {
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, "");
  const contact = `${id}-contact`;
  const shade = `${id}-shade`;

  return (
    <svg
      className={clsx("shisho-logo__icon", className)}
      fill="none"
      viewBox="0 0 48 48"
      xmlns="http://www.w3.org/2000/svg"
    >
      <defs>
        <linearGradient id={contact} x1="0" x2="0" y1="0" y2="1">
          <stop offset="0" stopColor="#000" stopOpacity="0" />
          <stop offset="1" stopColor="#000" stopOpacity="0.28" />
        </linearGradient>
        <linearGradient id={shade} x1="0" x2="0" y1="0" y2="1">
          <stop offset="0" stopColor="#000" stopOpacity="0" />
          <stop offset="1" stopColor="#000" stopOpacity="0.3" />
        </linearGradient>
      </defs>
      <rect fill="currentColor" height="4" rx="1" width="40" x="4" y="40" />
      <rect
        fill="#fff"
        height="1"
        opacity="0.28"
        rx="0.5"
        width="38"
        x="5"
        y="40"
      />
      <rect fill={`url(#${shade})`} height="4" rx="1" width="40" x="4" y="40" />
      {BOOKS.map(({ dim, h, w, x, y }) => (
        <g key={x} opacity={dim ? 0.7 : undefined}>
          <rect fill="currentColor" height={h} rx="1" width={w} x={x} y={y} />
          <rect
            fill="#fff"
            height={h - 2}
            opacity="0.28"
            rx="0.5"
            width="1"
            x={x}
            y={y + 1}
          />
          <rect
            fill="#fff"
            height="1"
            opacity="0.35"
            rx="0.5"
            width={w - 2}
            x={x + 1}
            y={y + 4}
          />
          <rect
            fill={`url(#${contact})`}
            height="7"
            rx="1"
            width={w}
            x={x}
            y="33"
          />
        </g>
      ))}
    </svg>
  );
}

export default function ShishoLogo({
  asLink = false,
  className,
  size = "md",
}: ShishoLogoProps): ReactNode {
  const content = (
    <>
      <ShelfIcon />
      <span className="shisho-logo__wordmark">
        Shisho
        <span className="shisho-logo__sup">司書</span>
      </span>
    </>
  );

  const classes = clsx(
    "shisho-logo",
    `shisho-logo--${size}`,
    asLink && "shisho-logo--link",
    className,
  );

  if (asLink) {
    return (
      <Link className={classes} to="/">
        {content}
      </Link>
    );
  }

  return <span className={classes}>{content}</span>;
}
