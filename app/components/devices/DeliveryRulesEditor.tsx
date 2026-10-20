import equal from "fast-deep-equal";
import { ArrowDown, ArrowUp, GripVertical, Loader2, X } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  SortableList,
  type DragHandleProps,
} from "@/components/ui/SortableList";
import {
  useDeliveryRules,
  useResetDeliveryRules,
  useUpdateDeliveryRules,
} from "@/hooks/queries/delivery";
import { toastRequestError } from "@/libraries/api";
import type { DeliveredFileType, FileType, Rule } from "@/types";
import { fileTypeLabel } from "@/utils/format";

// A rule being edited, with a client-side key that survives reordering so
// dnd-kit can track the row.
interface DraftRule extends Rule {
  key: string;
}

let draftKeyCounter = 0;
const withKeys = (rules: Rule[]): DraftRule[] =>
  rules.map((rule) => ({ ...rule, key: `rule-${++draftKeyCounter}` }));
const withoutKeys = (rules: DraftRule[]): Rule[] =>
  rules.map(({ source_file_type, delivered_file_type }) => ({
    source_file_type,
    delivered_file_type,
  }));

interface DeliveryRulesEditorProps {
  /** Reports whether the editor holds unsaved changes. */
  onHasChangesChange?: (hasChanges: boolean) => void;
}

// Editor for the current user's default Delivery Rules, which the web UI's
// main download button and OPDS signed in with the account password follow.
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
  const deliveredOptions = (source: FileType): DeliveredFileType[] =>
    options.find((o) => o.source_file_type === source)
      ?.delivered_file_types ?? [source];
  const unlistedSources = options
    .map((o) => o.source_file_type)
    .filter((source) => !draft.some((r) => r.source_file_type === source));

  const move = (index: number, offset: number) => {
    const next = [...draft];
    const [rule] = next.splice(index, 1);
    next.splice(index + offset, 0, rule);
    setDraft(next);
  };

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

      {draft.length === 0 ? (
        <p className="text-sm text-destructive" role="alert">
          Add at least one rule. With no rules, nothing would be delivered.
        </p>
      ) : (
        <SortableList<DraftRule>
          getItemId={(rule) => rule.key}
          items={draft}
          onReorder={setDraft}
          renderItem={(rule, index, drag: DragHandleProps) => {
            const label = fileTypeLabel(rule.source_file_type);
            return (
              <div className="flex items-center gap-2">
                <Button
                  aria-label={`Drag the ${label} rule`}
                  className="cursor-grab touch-none text-muted-foreground hover:text-foreground"
                  variant="unstyled"
                  {...drag.attributes}
                  {...drag.listeners}
                >
                  <GripVertical className="h-4 w-4" />
                </Button>
                <span className="w-14 text-sm font-medium">{label}</span>
                <span className="text-sm text-muted-foreground">as</span>
                <Select
                  onValueChange={(value) =>
                    setDraft(
                      draft.map((r) =>
                        r.key === rule.key
                          ? {
                              ...r,
                              delivered_file_type: value as DeliveredFileType,
                            }
                          : r,
                      ),
                    )
                  }
                  value={rule.delivered_file_type}
                >
                  <SelectTrigger
                    aria-label={`Deliver ${label} as`}
                    className="w-32"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {deliveredOptions(rule.source_file_type).map((type) => (
                      <SelectItem key={type} value={type}>
                        {fileTypeLabel(type)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <div className="ml-auto flex items-center">
                  <Button
                    aria-label={`Move the ${label} rule up`}
                    disabled={index === 0}
                    onClick={() => move(index, -1)}
                    size="icon-sm"
                    variant="ghost"
                  >
                    <ArrowUp className="h-4 w-4" />
                  </Button>
                  <Button
                    aria-label={`Move the ${label} rule down`}
                    disabled={index === draft.length - 1}
                    onClick={() => move(index, 1)}
                    size="icon-sm"
                    variant="ghost"
                  >
                    <ArrowDown className="h-4 w-4" />
                  </Button>
                  <Button
                    aria-label={`Remove the ${label} rule`}
                    onClick={() =>
                      setDraft(draft.filter((r) => r.key !== rule.key))
                    }
                    size="icon-sm"
                    variant="ghost"
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            );
          }}
        />
      )}

      {unlistedSources.length > 0 && (
        <Select
          onValueChange={(value) =>
            setDraft([
              ...draft,
              ...withKeys([
                {
                  source_file_type: value as FileType,
                  delivered_file_type: value as DeliveredFileType,
                },
              ]),
            ])
          }
          value=""
        >
          <SelectTrigger aria-label="Add a rule" className="w-full sm:w-56">
            <SelectValue placeholder="Add a rule for…" />
          </SelectTrigger>
          <SelectContent>
            {unlistedSources.map((source) => (
              <SelectItem key={source} value={source}>
                {fileTypeLabel(source)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

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
