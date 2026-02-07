import Link from "@docusaurus/Link";
import clsx from "clsx";
import type { ReactNode } from "react";

type LogoSize = "sm" | "md" | "lg";

interface ShishoLogoProps {
  asLink?: boolean;
  className?: string;
  size?: LogoSize;
}

function ShelfIcon({ className }: { className?: string }) {
  return (
    <svg
      className={clsx("shisho-logo__icon", className)}
      fill="none"
      viewBox="0 0 48 48"
      xmlns="http://www.w3.org/2000/svg"
    >
      <rect fill="currentColor" height="4" rx="1" width="40" x="4" y="40" />
      <rect fill="currentColor" height="28" rx="1" width="7" x="8" y="12" />
      <rect
        fill="currentColor"
        height="32"
        opacity="0.7"
        rx="1"
        width="6"
        x="17"
        y="8"
      />
      <rect fill="currentColor" height="24" rx="1" width="8" x="25" y="16" />
      <rect
        fill="currentColor"
        height="30"
        opacity="0.7"
        rx="1"
        width="5"
        x="35"
        y="10"
      />
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
