import { AlertTriangle, Copy } from "lucide-react";
import { useId } from "react";

import { Button } from "@/components/ui/button";
import { copyTextWithToast } from "@/utils/copyWithToast";
import { isLoopbackHost, pageHostname } from "@/utils/location";

/**
 * A value in monospace with a copy button, for a URL or credential someone
 * passes on. A long URL wraps rather than scrolling inside a field, so it can
 * be read on a phone. `label` names the value for screen readers and the
 * button ("Copy invite link"). Pass `url` for a URL built from the page's
 * address: while the page is open at localhost or another loopback address,
 * it's marked as one other devices can't reach.
 */
const CopyField = ({
  label,
  value,
  url = false,
}: {
  label: string;
  value: string;
  url?: boolean;
}) => {
  const unreachable = url && isLoopbackHost(pageHostname());
  const noteId = useId();
  return (
    <div className="space-y-1">
      <div className="flex items-start gap-2">
        <code
          aria-describedby={unreachable ? noteId : undefined}
          aria-label={label}
          className="min-w-0 flex-1 break-all rounded-md border border-input px-3 py-2 font-mono text-sm"
        >
          {value}
        </code>
        <Button
          aria-label={`Copy ${label.toLowerCase()}`}
          onClick={() => copyTextWithToast(value)}
          variant="outline"
        >
          <Copy className="h-4 w-4" />
        </Button>
      </div>
      {unreachable && (
        <p
          className="flex items-center gap-1.5 text-xs text-amber-700 dark:text-amber-400"
          id={noteId}
        >
          <AlertTriangle aria-hidden="true" className="h-3.5 w-3.5 shrink-0" />
          Other devices can&apos;t reach this address.
        </p>
      )}
    </div>
  );
};

export default CopyField;
