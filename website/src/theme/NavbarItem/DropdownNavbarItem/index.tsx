import DropdownNavbarItemDesktop from "./Desktop";
import DropdownNavbarItemMobile from "@theme-original/NavbarItem/DropdownNavbarItem/Mobile";
import type { ReactNode } from "react";

interface DropdownNavbarItemProps {
  mobile?: boolean;
  [key: string]: unknown;
}

export default function DropdownNavbarItem({
  mobile = false,
  ...props
}: DropdownNavbarItemProps): ReactNode {
  const Comp = mobile ? DropdownNavbarItemMobile : DropdownNavbarItemDesktop;
  return <Comp {...props} />;
}
