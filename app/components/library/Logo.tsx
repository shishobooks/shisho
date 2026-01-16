import { Link } from "react-router-dom";

import { cn } from "@/libraries/utils";

const ShelfIcon = ({ className }: { className?: string }) => (
  <svg
    className={cn("text-primary dark:text-violet-300", className)}
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
            "align-super font-normal text-primary dark:text-violet-300 ml-0.5",
            superscriptSizeClasses[size],
          )}
        >
          司書
        </span>
      </span>
    </>
  );

  const baseClasses = cn(
    "inline-flex items-center font-bold uppercase tracking-wider text-foreground",
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
