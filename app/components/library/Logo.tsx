import { useId } from "react";
import { Link } from "react-router-dom";

import { cn } from "@/libraries/utils";

// Shelf mark: four book spines on a shelf. Each book is a flat base rect plus
// three subtle overlays (spine highlight, title band, contact shadow) so the
// icon reads with depth at large sizes while degrading to the plain silhouette
// at 16px. Gradient ids are namespaced per instance because the logo renders
// more than once per page. The useId output is stripped to id-safe characters
// for the React 18 `:r0:` format; React 19 ids are already safe.
const BOOKS = [
  { dim: false, h: 28, w: 7, x: 8, y: 12 },
  { dim: true, h: 32, w: 6, x: 17, y: 8 },
  { dim: false, h: 24, w: 8, x: 25, y: 16 },
  { dim: true, h: 30, w: 5, x: 35, y: 10 },
];

const ShelfIcon = ({ className }: { className?: string }) => {
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, "");
  const contact = `${id}-contact`;
  const shade = `${id}-shade`;

  return (
    <svg
      className={cn("text-primary", className)}
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
};

interface LogoProps {
  asLink?: boolean;
  className?: string;
  size?: "sm" | "md" | "lg";
}

const Logo = ({ asLink = false, className, size = "md" }: LogoProps) => {
  const sizeClasses = {
    sm: "text-lg",
    md: "text-xl",
    lg: "text-2xl",
  };

  const superscriptSizeClasses = {
    sm: "text-[10px]",
    md: "text-xs",
    lg: "text-sm",
  };

  const iconSizeClasses = {
    sm: "w-4 h-4",
    md: "w-5 h-5",
    lg: "w-6 h-6",
  };

  const content = (
    <>
      <ShelfIcon className={cn(iconSizeClasses[size], "mr-1")} />
      <span>
        Shisho
        <span
          className={cn(
            "align-super font-normal text-primary ml-0.5",
            superscriptSizeClasses[size],
          )}
        >
          司書
        </span>
      </span>
    </>
  );

  const baseClasses = cn(
    "inline-flex items-center font-logo font-bold uppercase tracking-wider text-foreground",
    sizeClasses[size],
    className,
  );

  if (asLink) {
    return (
      <Link
        className={cn(baseClasses, "hover:opacity-80 transition-opacity")}
        to="/"
      >
        {content}
      </Link>
    );
  }

  return <span className={baseClasses}>{content}</span>;
};

export default Logo;
