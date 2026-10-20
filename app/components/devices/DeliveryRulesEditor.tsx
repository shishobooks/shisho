import equal from "fast-deep-equal";
import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import RulesListEditor from "@/components/devices/RulesListEditor";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useDeliveryRules,
  useResetDeliveryRules,
  useUpdateDeliveryRules,
} from "@/hooks/queries/delivery";
import { toastRequestError } from "@/libraries/api";
import type { Rule } from "@/types";
import { withKeys, withoutKeys, type DraftRule } from "@/utils/delivery";

interface DeliveryRulesEditorProps {
  /** Reports whether the editor holds unsaved changes. */
  onHasChangesChange?: (hasChanges: boolean) => void;
}

// Editor for the current user's default Delivery Rules, which the web UI's
// main download button, OPDS signed in with the account password, and every
// Device set to follow the default use.
// While the default follows Shisho's built-in rules, the editor starts from
// a copy of them, and saving makes the copy the user's own.
const DeliveryRulesEditor = ({
  onHasChangesChange,
}: DeliveryRulesEditorProps) => {
  const rulesQuery = useDeliveryRules();
  const updateRules = useUpdateDeliveryRules();
  const resetRules = useResetDeliveryRules();
  const [draft, setDraft] = useState<DraftRule[] | null>(null);
  const [initial, setInitial] = useState<Rule[] | null>(null);

  // Initialize once, so a background refetch cannot overwrite edits.
  useEffect(() => {
    if (rulesQuery.data && initial === null) {
      setInitial(rulesQuery.data.rules);
      setDraft(withKeys(rulesQuery.data.rules));
    }
  }, [rulesQuery.data, initial]);

  const hasChanges =
    draft !== null && initial !== null && !equal(withoutKeys(draft), initial);
  useEffect(() => {
    onHasChangesChange?.(hasChanges);
  }, [hasChanges, onHasChangesChange]);

  if (rulesQuery.error && !rulesQuery.data) {
    return (
      <QueryError fallback="Failed to load Delivery Rules" query={rulesQuery} />
    );
  }
  if (!rulesQuery.data || draft === null) {
    return <LoadingSpinner />;
  }

  const { options, follows_built_in: followsBuiltIn } = rulesQuery.data;

  const applySaved = (rules: Rule[]) => {
    setInitial(rules);
    setDraft(withKeys(rules));
  };

  const handleSave = async () => {
    try {
      const saved = await updateRules.mutateAsync({
        rules: withoutKeys(draft),
      });
      applySaved(saved.rules);
      toast.success("Delivery Rules saved");
    } catch (error) {
      toastRequestError(error, "Failed to save Delivery Rules");
    }
  };

  const handleReset = async () => {
    try {
      const reset = await resetRules.mutateAsync();
      applySaved(reset.rules);
      toast.success("Delivery Rules reset to Shisho's defaults");
    } catch (error) {
      toastRequestError(error, "Failed to reset Delivery Rules");
    }
  };

  const isPending = updateRules.isPending || resetRules.isPending;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        {followsBuiltIn ? (
          <Badge variant="secondary">Following Shisho&apos;s defaults</Badge>
        ) : (
          <Badge variant="secondary">Customized</Badge>
        )}
      </div>

      <RulesListEditor onChange={setDraft} options={options} rules={draft} />

      <div className="flex flex-wrap items-center gap-2">
        <Button
          disabled={!hasChanges || draft.length === 0 || isPending}
          onClick={handleSave}
          size="sm"
        >
          {updateRules.isPending && (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          )}
          Save
        </Button>
        {!followsBuiltIn && (
          <Button
            disabled={isPending}
            onClick={handleReset}
            size="sm"
            variant="outline"
          >
            {resetRules.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Reset to Shisho&apos;s defaults
          </Button>
        )}
        {hasChanges && initial !== null && (
          <Button
            disabled={isPending}
            onClick={() => applySaved(initial)}
            size="sm"
            variant="ghost"
          >
            Discard changes
          </Button>
        )}
      </div>
    </div>
  );
};

export default DeliveryRulesEditor;
