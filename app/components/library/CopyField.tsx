import { Copy } from "lucide-react";

import { Button } from "@/components/ui/button";
import { copyTextWithToast } from "@/utils/copyWithToast";

/**
 * A value in monospace with a copy button, for a URL or credential someone
 * passes on. A long URL wraps rather than scrolling inside a field, so it can
 * be read on a phone. `label` names the value for screen readers and the
 * button ("Copy invite link").
 */
const CopyField = ({ label, value }: { label: string; value: string }) => (
  <div className="flex items-start gap-2">
    <code
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
);

export default CopyField;
