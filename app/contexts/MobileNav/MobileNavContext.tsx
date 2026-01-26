import { createContext, useCallback, useState, type ReactNode } from "react";

interface MobileNavContextValue {
  isOpen: boolean;
  open: () => void;
  close: () => void;
  toggle: () => void;
}

const MobileNavContext = createContext<MobileNavContextValue | null>(null);

export const MobileNavProvider = ({ children }: { children: ReactNode }) => {
  const [isOpen, setIsOpen] = useState(false);

  const open = useCallback(() => setIsOpen(true), []);
  const close = useCallback(() => setIsOpen(false), []);
  const toggle = useCallback(() => setIsOpen((prev) => !prev), []);

  return (
    <MobileNavContext.Provider value={{ isOpen, open, close, toggle }}>
      {children}
    </MobileNavContext.Provider>
  );
};

// Exported from separate file to satisfy react-refresh/only-export-components
export { MobileNavContext };
