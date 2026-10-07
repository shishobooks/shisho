import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import * as React from "react";

import { cn } from "@/libraries/utils";

// Every variant, including `unstyled`, keeps these: the pointer cursor, the
// disabled treatment, and the keyboard focus ring.
const buttonBase =
  "cursor-pointer focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50";

// Layout and typography for every variant except `unstyled`, which leaves
// them to the caller (rows, tiles, tap zones).
const buttonLayout =
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0";

const buttonStyles = cva(buttonBase, {
  variants: {
    variant: {
      default: "bg-primary text-primary-foreground shadow hover:bg-primary/90",
      destructive:
        "bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90",
      outline:
        "border border-input bg-background shadow-sm hover:bg-accent hover:text-accent-foreground",
      secondary:
        "bg-secondary text-secondary-foreground shadow-sm hover:bg-secondary/80",
      ghost: "hover:bg-accent hover:text-accent-foreground",
      link: "text-primary underline-offset-4 hover:underline",
      unstyled: "",
    },
    size: {
      default: "h-9 px-4 py-2",
      sm: "h-8 rounded-md px-3 text-xs",
      lg: "h-10 rounded-md px-8",
      icon: "h-9 w-9",
      "icon-sm": "h-7 w-7",
      "icon-xs": "h-5 w-5",
      inline: "h-auto p-0",
      none: "",
    },
  },
});

type ButtonVariantProps = VariantProps<typeof buttonStyles>;

// `link` defaults to `inline` (no height or padding) and `unstyled` to `none`,
// so neither needs `h-auto p-0` overrides at the call site.
const defaultSize = (
  variant: ButtonVariantProps["variant"],
): NonNullable<ButtonVariantProps["size"]> => {
  if (variant === "link") return "inline";
  if (variant === "unstyled") return "none";
  return "default";
};

const buttonVariants = ({
  variant = "default",
  size,
  className,
}: ButtonVariantProps & { className?: string } = {}) =>
  cn(
    variant !== "unstyled" && buttonLayout,
    buttonStyles({ variant, size: size ?? defaultSize(variant) }),
    className,
  );

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>, ButtonVariantProps {
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, type, ...props }, ref) => {
    if (asChild) {
      return (
        <Slot
          className={buttonVariants({ variant, size, className })}
          ref={ref}
          {...props}
        />
      );
    }
    return (
      <button
        className={buttonVariants({ variant, size, className })}
        ref={ref}
        type={type ?? "button"}
        {...props}
      />
    );
  },
);
Button.displayName = "Button";

export { Button, buttonVariants };
