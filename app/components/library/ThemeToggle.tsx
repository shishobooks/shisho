import { Moon, Sun } from "lucide-react";

import { useTheme } from "@/components/contexts/Theme/context";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

const ThemeToggle = () => {
  const { theme, setTheme } = useTheme();

  const handleToggle = () => {
    if (theme === "light") {
      setTheme("dark");
    } else if (theme === "dark") {
      setTheme("light");
    } else {
      // If system, toggle to light or dark based on current system preference
      const isDarkMode = window.matchMedia(
        "(prefers-color-scheme: dark)",
      ).matches;
      setTheme(isDarkMode ? "light" : "dark");
    }
  };

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            className="h-9 w-9 relative"
            onClick={handleToggle}
            size="icon"
            variant="ghost"
          >
            <Sun className="h-4 w-4 rotate-0 scale-100 transition-all dark:-rotate-90 dark:scale-0" />
            <Moon className="absolute h-4 w-4 rotate-90 scale-0 transition-all dark:rotate-0 dark:scale-100" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <p>Toggle theme</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
};

export default ThemeToggle;
