// aria-invalid tells a screen reader that a field is wrong but not why. The
// message a sighted user reads next to the field is announced with it only
// when the field points at it with aria-describedby (or aria-errormessage).
// This rule flags any element with aria-invalid, other than a literal false
// or `{undefined}`, that has no spread props (the rule cannot see what a
// spread passes) and no aria-describedby or aria-errormessage that is
// non-empty and not `{undefined}`. See "Enforced by checks" in
// app/AGENTS.md, and app/eslint-rules.test.ts for the pinned cases and known
// gaps.

import {
  attribute,
  hasSpread,
  hasValue,
  isUndefinedValue,
  literalValue,
} from "./control-has-name.js";

const MESSAGE_ATTRIBUTES = ["aria-describedby", "aria-errormessage"];

const isMarkedInvalid = (opening) => {
  if (!attribute(opening, "aria-invalid")) return false;
  if (isUndefinedValue(opening, "aria-invalid")) return false;
  return literalValue(opening, "aria-invalid") !== "false";
};

const create = (context) => ({
  JSXOpeningElement(opening) {
    if (!isMarkedInvalid(opening) || hasSpread(opening)) return;
    const pointsAtMessage = MESSAGE_ATTRIBUTES.some(
      (name) => hasValue(opening, name) && !isUndefinedValue(opening, name),
    );
    if (pointsAtMessage) return;
    context.report({ node: opening, messageId: "noMessage" });
  },
});

export default {
  meta: {
    type: "problem",
    docs: {
      description:
        "Require every element marked aria-invalid to point at the message that says why.",
    },
    messages: {
      noMessage:
        'Tie this invalid field to its error message: give the message an id (per row when the field repeats), and point at it with aria-describedby={error ? id : undefined}. Add role="alert" to the message when it appears after an action such as Save (see app/AGENTS.md).',
    },
    schema: [],
  },
  create,
};
