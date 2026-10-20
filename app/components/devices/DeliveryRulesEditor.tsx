import { useQueryClient } from "@tanstack/react-query";
import equal from "fast-deep-equal";
import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import RulesListEditor from "@/components/devices/RulesListEditor";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  useDeliveryRules,
  useResetDeliveryRules,
  useUpdateDeliveryRules,
} from "@/hooks/queries/delivery";
import { fetchDeliveryPreview, fetchDevices } from "@/hooks/queries/devices";
import { toastRequestError } from "@/libraries/api";
import type { Rule } from "@/types";
import {
  DeviceTypeKobo,
  RulesModeCustom,
  RulesModeDefault,
  type DeviceResponse,
} from "@/types/generated/devices";
import { withKeys, withoutKeys, type DraftRule } from "@/utils/delivery";
import { formatCount } from "@/utils/format";

// The rules a Device following the default would follow under rules: the
// ones whose delivered File Type its type can receive.
const followedBy = (device: DeviceResponse, rules: Rule[]): Rule[] =>
  rules.filter((rule) =>
    device.delivery_rule_options
      .find((o) => o.source_file_type === rule.source_file_type)
      ?.delivered_file_types.includes(rule.delivered_file_type),
  );

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
  const queryClient = useQueryClient();
  // A save held back until the reader confirms what it does to books on a
  // Kobo following the default.
  const [pending, setPending] = useState<{
    message: string;
    run: () => Promise<void>;
  } | null>(null);
  const [checking, setChecking] = useState(false);
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

  // koboWarning says what rules, as the new default, would do to books on
  // each Kobo following the default: a switch between KePub and EPUB loses
  // their place and highlights, and a move to another edition may reopen at
  // the start (ADR 0012). It is empty when nothing would.
  const koboWarning = async (rules: Rule[]): Promise<string> => {
    const kobos = (await fetchDevices(queryClient)).filter(
      (d) =>
        d.device_type === DeviceTypeKobo &&
        d.delivery_rules_mode === RulesModeDefault,
    );
    let retyped = 0;
    let moved = 0;
    for (const kobo of kobos) {
      const followed = followedBy(kobo, rules);
      if (followed.length === 0) continue;
      try {
        const preview = await fetchDeliveryPreview(queryClient, kobo.id, {
          mode: RulesModeCustom,
          rules: followed,
        });
        retyped += preview.retyped_books;
        moved += preview.moved_books;
      } catch {
        // The warning is advice; the save reports its own error.
      }
    }
    const parts: string[] = [];
    if (retyped > 0) {
      parts.push(
        `${formatCount(retyped, "book")} on a Kobo following your default will switch between KePub and EPUB. The Kobo removes each one and adds it again as a new book, which loses its reading position and highlights.`,
      );
    }
    if (moved > 0) {
      parts.push(
        `${formatCount(moved, "book")} on a Kobo following your default will move to a different edition at its next sync, and may reopen at the start.`,
      );
    }
    return parts.join(" ");
  };

  // confirmFirst runs a save, first asking when it would change books on a
  // Kobo following the default.
  const confirmFirst = async (rules: Rule[], run: () => Promise<void>) => {
    setChecking(true);
    let message = "";
    try {
      message = await koboWarning(rules);
    } catch {
      // The warning is advice; the save reports its own error.
    } finally {
      setChecking(false);
    }
    if (message) {
      setPending({ message, run });
      return;
    }
    await run();
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

  // Shisho's built-in rules deliver each File Type as itself, in the
  // options' order.
  const builtIn: Rule[] = options.map((o) => ({
    source_file_type: o.source_file_type,
    delivered_file_type: o.source_file_type,
  }));

  const isPending = updateRules.isPending || resetRules.isPending || checking;

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
          onClick={() => confirmFirst(withoutKeys(draft), handleSave)}
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
            onClick={() => confirmFirst(builtIn, handleReset)}
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

      <ConfirmDialog
        confirmLabel="Save rules"
        description={pending?.message ?? ""}
        isPending={updateRules.isPending || resetRules.isPending}
        onConfirm={async () => {
          if (!pending) return;
          try {
            await pending.run();
          } finally {
            setPending(null);
          }
        }}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
        open={pending !== null}
        title="Change books on your Kobo?"
      />
    </div>
  );
};

export default DeliveryRulesEditor;
