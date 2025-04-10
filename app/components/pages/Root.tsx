import { Outlet } from "react-router-dom";

import { Toaster } from "@/components/ui/sonner";

const Root = () => {
  return (
    <div className="flex bg-neutral-50 font-sans dark:bg-neutral-800">
      <div className="h-screen grow overflow-scroll">
        <Outlet />
      </div>
      <Toaster richColors theme="light" />
    </div>
  );
};

export default Root;
