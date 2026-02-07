import NavbarItem from "@theme/NavbarItem";
import NavbarNavLink from "@theme/NavbarItem/NavbarNavLink";
import clsx from "clsx";
import { ChevronDown } from "lucide-react";
import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
} from "react";

interface DropdownNavbarItemDesktopProps {
  items: Record<string, unknown>[];
  position?: "left" | "right";
  className?: string;
  to?: string;
  label?: string;
  children?: ReactNode;
  [key: string]: unknown;
}

export default function DropdownNavbarItemDesktop({
  items,
  position,
  className,
  ...props
}: DropdownNavbarItemDesktopProps) {
  const dropdownRef = useRef<HTMLDivElement | null>(null);
  const [showDropdown, setShowDropdown] = useState(false);

  useEffect(() => {
    const handleClickOutside = (
      event: MouseEvent | TouchEvent | FocusEvent,
    ) => {
      if (!dropdownRef.current) return;
      const target = event.target;
      if (target instanceof Node && dropdownRef.current.contains(target))
        return;
      setShowDropdown(false);
    };

    document.addEventListener("mousedown", handleClickOutside);
    document.addEventListener("touchstart", handleClickOutside);
    document.addEventListener("focusin", handleClickOutside);

    return () => {
      document.removeEventListener("mousedown", handleClickOutside);
      document.removeEventListener("touchstart", handleClickOutside);
      document.removeEventListener("focusin", handleClickOutside);
    };
  }, []);

  return (
    <div
      className={clsx("navbar__item", "dropdown", "dropdown--hoverable", {
        "dropdown--right": position === "right",
        "dropdown--show": showDropdown,
      })}
      ref={dropdownRef}
    >
      <NavbarNavLink
        aria-expanded={showDropdown}
        aria-haspopup="true"
        className={clsx("navbar__link", className)}
        href={props.to ? undefined : "#"}
        onClick={
          props.to
            ? undefined
            : (e: MouseEvent<HTMLAnchorElement>) => e.preventDefault()
        }
        onKeyDown={(e: KeyboardEvent<HTMLAnchorElement>) => {
          if (e.key === "Enter") {
            e.preventDefault();
            setShowDropdown(!showDropdown);
          }
        }}
        role="button"
        {...props}
      >
        <span>{props.children ?? props.label}</span>
        <ChevronDown
          aria-hidden
          className="navbar-dropdown-chevron"
          size={16}
          strokeWidth={2.1}
        />
      </NavbarNavLink>
      <ul className="dropdown__menu">
        {items.map((childItemProps, i) => (
          <NavbarItem
            activeClassName="dropdown__link--active"
            isDropdownItem
            key={i}
            {...childItemProps}
          />
        ))}
      </ul>
    </div>
  );
}
