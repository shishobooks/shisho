import { Link } from "react-router-dom";

import { cn } from "@/libraries/utils";

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

  const content = (
    <>
      Shisho
      <span
        className={cn(
          "align-super font-normal text-primary dark:text-violet-300 ml-0.5",
          superscriptSizeClasses[size],
        )}
      >
        司書
      </span>
    </>
  );

  const baseClasses = cn(
    "font-bold uppercase tracking-wider text-foreground",
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
