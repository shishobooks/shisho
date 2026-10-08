// A red border is how a sighted user learns a field is wrong; a screen reader
// learns it only from aria-invalid. This rule flags a form control (the ui
// kit's Input, Textarea, and SelectTrigger, and raw input, select, and
// textarea) whose className turns on a destructive or red border
// conditionally (inside a ternary, a && or ||, or a cn() object key) and that
// has no aria-invalid (`{undefined}` counts as none) and no spread props (the rule cannot see what a spread
// passes). The border counts with an important modifier (`!border-red-500`,
// `border-red-500!`) and behind breakpoint or dark: variants. A border that
// is always red is styling, not an error state, so it is not flagged, and
// neither is a state variant such as aria-invalid:, focus:, or hover: that
// only applies in some other state. See "Enforced by checks" in
// app/AGENTS.md, and app/eslint-rules.test.ts for the pinned cases and known
// gaps.

import {
  attribute,
  elementName,
  hasSpread,
  isUndefinedValue,
} from "./control-has-name.js";

const CONTROLS = new Set([
  "Input",
  "Textarea",
  "SelectTrigger",
  "input",
  "select",
  "textarea",
]);

// border-destructive, border-red-500, their one-side forms (border-l-red-500,
// border-x-destructive), and their opacity and important forms, bare or
// behind breakpoint and dark: variants only.
const ERROR_BORDER =
  /(^|\s)((sm|md|lg|xl|2xl|dark):)*!?border-([xytrblse]-)?(destructive|red-\d+)(\/\d+)?!?(\s|$)/;

// Whether any string the className can only hold under some condition has an
// error border.
const hasConditionalErrorBorder = (value) => {
  let found = false;
  const visit = (node, conditional) => {
    if (found || !node || typeof node !== "object") return;
    if (conditional) {
      if (node.type === "Literal" && typeof node.value === "string") {
        found = ERROR_BORDER.test(node.value);
        if (found) return;
      } else if (node.type === "TemplateElement") {
        found = ERROR_BORDER.test(node.value.raw);
        if (found) return;
      }
    }
    for (const key of Object.keys(node)) {
      if (key === "parent") continue;
      const childConditional =
        conditional ||
        (node.type === "ConditionalExpression" &&
          (key === "consequent" || key === "alternate")) ||
        (node.type === "LogicalExpression" && key === "right") ||
        (node.type === "Property" && key === "key");
      const child = node[key];
      if (Array.isArray(child)) {
        child.forEach((item) => visit(item, childConditional));
      } else if (child && typeof child.type === "string") {
        visit(child, childConditional);
      }
    }
  };
  visit(value, false);
  return found;
};

const create = (context) => ({
  JSXOpeningElement(opening) {
    if (!CONTROLS.has(elementName(opening))) return;
    const className = attribute(opening, "className");
    if (!className?.value || className.value.type === "Literal") return;
    const marksInvalid =
      attribute(opening, "aria-invalid") &&
      !isUndefinedValue(opening, "aria-invalid");
    if (marksInvalid || hasSpread(opening)) return;
    if (hasConditionalErrorBorder(className.value)) {
      context.report({ node: opening, messageId: "notInvalid" });
    }
  },
});

export default {
  meta: {
    type: "problem",
    docs: {
      description:
        "Require a form control that turns its border red on error to say so with aria-invalid.",
    },
    messages: {
      notInvalid:
        "This control shows an error only by a red border. Set aria-invalid={error ? true : undefined} under the same condition, and tie it to a text message with aria-describedby (see app/AGENTS.md).",
    },
    schema: [],
  },
  create,
};
