import { ArrowDown, ArrowUp, GripVertical, X } from "lucide-react";

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
import type { DeliveredFileType, FileType, RuleOption } from "@/types";
import { withKeys, type DraftRule } from "@/utils/delivery";
import { fileTypeLabel } from "@/utils/format";

interface RulesListEditorProps {
  rules: DraftRule[];
  onChange: (rules: DraftRule[]) => void;
  /** Every source File Type a rule can name, with what it can become. */
  options: RuleOption[];
}

// RulesListEditor edits an ordered list of Delivery Rules: reorder, choose
// what each source File Type is delivered as, remove a rule, or add one for
// a source File Type the list leaves out. Saving is the caller's.
const RulesListEditor = ({
  rules,
  onChange,
  options,
}: RulesListEditorProps) => {
  const deliveredOptions = (source: FileType): DeliveredFileType[] =>
    options.find((o) => o.source_file_type === source)
      ?.delivered_file_types ?? [source];
  const unlistedSources = options
    .map((o) => o.source_file_type)
    .filter((source) => !rules.some((r) => r.source_file_type === source));

  const move = (index: number, offset: number) => {
    const next = [...rules];
    const [rule] = next.splice(index, 1);
    next.splice(index + offset, 0, rule);
    onChange(next);
  };

  return (
    <div className="space-y-4">
      {rules.length === 0 ? (
        <p className="text-sm text-destructive" role="alert">
          Add at least one rule. With no rules, nothing would be delivered.
        </p>
      ) : (
        <SortableList<DraftRule>
          getItemId={(rule) => rule.key}
          items={rules}
          onReorder={onChange}
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
                    onChange(
                      rules.map((r) =>
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
                    disabled={index === rules.length - 1}
                    onClick={() => move(index, 1)}
                    size="icon-sm"
                    variant="ghost"
                  >
                    <ArrowDown className="h-4 w-4" />
                  </Button>
                  <Button
                    aria-label={`Remove the ${label} rule`}
                    onClick={() =>
                      onChange(rules.filter((r) => r.key !== rule.key))
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
          onValueChange={(value) => {
            const source = value as FileType;
            onChange([
              ...rules,
              ...withKeys([
                {
                  source_file_type: source,
                  // Its own File Type when allowed, otherwise the first
                  // one it can become.
                  delivered_file_type: deliveredOptions(source)[0],
                },
              ]),
            ]);
          }}
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
    </div>
  );
};

export default RulesListEditor;
