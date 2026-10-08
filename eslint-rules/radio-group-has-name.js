// A screen reader announces a radio group's name before its options, so the
// options ("Copy", "Move") make sense without the heading a sighted user
// reads above them. A visible Label names nothing unless it is linked. This
// rule flags RadioGroup and any element with role="radiogroup" that has none
// of: a non-empty aria-label or aria-labelledby that is not `{undefined}`,
// spread props (the rule cannot see what a spread passes), or an enclosing
// fieldset (whose legend names it). See "Enforced by checks" in
// app/AGENTS.md, and app/eslint-rules.test.ts for the pinned cases and known
// gaps.

import {
  elementName,
  hasSpread,
  hasValue,
  isUndefinedValue,
  literalValue,
} from "./control-has-name.js";

const NAME_ATTRIBUTES = ["aria-label", "aria-labelledby"];

const isRadioGroup = (opening) =>
  elementName(opening) === "RadioGroup" ||
  literalValue(opening, "role") === "radiogroup";

const insideFieldset = (node) => {
  for (let parent = node.parent; parent; parent = parent.parent) {
    if (
      parent.type === "JSXElement" &&
      elementName(parent.openingElement) === "fieldset"
    ) {
      return true;
    }
  }
  return false;
};

const create = (context) => ({
  JSXElement(node) {
    const opening = node.openingElement;
    if (!isRadioGroup(opening) || hasSpread(opening)) return;
    const named = NAME_ATTRIBUTES.some(
      (name) => hasValue(opening, name) && !isUndefinedValue(opening, name),
    );
    if (named || insideFieldset(node)) return;
    context.report({ node: opening, messageId: "unnamed" });
  },
});

export default {
  meta: {
    type: "problem",
    docs: {
      description:
        "Require every radio group to have an accessible name, so its options make sense on their own.",
    },
    messages: {
      unnamed:
        'Name this radio group: aria-labelledby pointing at its visible heading or Label (useId() when the component renders more than once), or an aria-label ("Rescan mode") when there is none (see app/AGENTS.md).',
    },
    schema: [],
  },
  create,
};
