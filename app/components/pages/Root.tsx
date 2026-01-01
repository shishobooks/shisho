import { Outlet } from "react-router-dom";

import { Toaster } from "@/components/ui/sonner";

const Root = () => {
  return (
    <div className="flex bg-background font-sans min-h-screen">
      <div className="w-full">
        <Outlet />
      </div>
      <Toaster richColors />
    </div>
  );
};

export default Root;
