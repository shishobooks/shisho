import { Menu, Search, Settings, X } from "lucide-react";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";

import {
  TOP_NAV_INNER,
  TOP_NAV_ROW,
  TOP_NAV_WRAPPER,
} from "@/components/layout/topNavClasses";
import UserMenu from "@/components/layout/UserMenu";
import GlobalSearch from "@/components/library/GlobalSearch";
import LibraryListPicker from "@/components/library/LibraryListPicker";
import Logo from "@/components/library/Logo";
import { ResyncButton } from "@/components/library/ResyncButton";
import { useAdminNavItems } from "@/components/pages/useAdminNavItems";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useMobileNav } from "@/contexts/MobileNav";
import { useAuth } from "@/hooks/useAuth";
import { useCan } from "@/hooks/useCan";
import { cn } from "@/libraries/utils";

const TopNav = () => {
  const { libraryId } = useParams();
  const { demoMode } = useAuth();
  const { toggle } = useMobileNav();
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false);

  // The gear opens the first settings page the role may see.
  const canAccessAdmin = useAdminNavItems().some((item) => item.show);

  // GlobalSearch searches the current library and needs Books Read; without
  // either there is nothing for the mobile toggle to open.
  const canReadBooks = useCan("books:read");
  const canSearch = Boolean(libraryId) && canReadBooks;

  // The button reads the latest scan (Jobs Read) and starts one (Jobs Write).
  const canResync = useCan(["jobs:read", "jobs:write"]);

  return (
    <div className={TOP_NAV_WRAPPER}>
      <div className={TOP_NAV_INNER}>
        <div className={TOP_NAV_ROW}>
          {/* Left section */}
          <div className="flex items-center gap-2 md:gap-8">
            {/* Mobile hamburger menu */}
            <Button
              aria-label="Open navigation menu"
              className="md:hidden h-9 w-9 -ml-1"
              onClick={toggle}
              size="icon"
              variant="ghost"
            >
              <Menu className="h-5 w-5" />
            </Button>

            {/* Logo - hidden on mobile when search is open */}
            <div
              className={cn(
                "transition-opacity duration-200",
                mobileSearchOpen && "hidden sm:block",
              )}
            >
              <Logo asLink />
            </div>

            {/* Library/List Picker - hidden on mobile, shown on tablet+ */}
            <div className="hidden sm:flex items-center gap-1">
              <LibraryListPicker />
              {libraryId && canResync && (
                <ResyncButton libraryId={Number(libraryId)} />
              )}
            </div>
          </div>

          {/* Right section */}
          <div className="flex items-center gap-1 md:gap-4">
            {/* Mobile search toggle */}
            {canSearch && (
              <Button
                aria-label={mobileSearchOpen ? "Close search" : "Open search"}
                className="md:hidden h-9 w-9"
                onClick={() => setMobileSearchOpen(!mobileSearchOpen)}
                size="icon"
                variant="ghost"
              >
                {mobileSearchOpen ? (
                  <X className="h-5 w-5" />
                ) : (
                  <Search className="h-5 w-5" />
                )}
              </Button>
            )}

            {/* Desktop search */}
            <div className="hidden md:block">
              <GlobalSearch />
            </div>

            {canAccessAdmin && !demoMode && (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      asChild
                      className="h-9 w-9"
                      size="icon"
                      variant="ghost"
                    >
                      <Link to="/settings">
                        <Settings className="h-4 w-4" />
                      </Link>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>Global Settings</p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            )}
            <UserMenu />
          </div>
        </div>

        {/* Mobile search bar - expandable */}
        <div
          className={cn(
            "md:hidden overflow-hidden transition-all duration-200 ease-out",
            mobileSearchOpen ? "max-h-16 pb-3" : "max-h-0",
          )}
        >
          <GlobalSearch fullWidth onClose={() => setMobileSearchOpen(false)} />
        </div>
      </div>
    </div>
  );
};

export default TopNav;
