import * as React from "react";
import { DayPicker } from "react-day-picker";

import { cn } from "@/libraries/utils";

export type CalendarProps = React.ComponentProps<typeof DayPicker>;

function Calendar({ className, ...props }: CalendarProps) {
  return <DayPicker className={cn("rdp-root", className)} {...props} />;
}
Calendar.displayName = "Calendar";

export { Calendar };
