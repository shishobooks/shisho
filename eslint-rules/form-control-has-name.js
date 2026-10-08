// A screen reader announces a form control by its accessible name. A
// placeholder is not one (it disappears once the user types), a visible label
// names nothing unless it is linked to its control, and the value a combobox
// shows is its value, not its name. This rule flags form controls (the ui
// kit's Input, Textarea, Checkbox, Switch, RadioGroupItem, Slider,
// SelectTrigger, and CommandInput, raw input, select, and textarea, and a
// Button with role="combobox") that have none of:
//
// - a non-empty aria-label or aria-labelledby, or spread props (the rule
//   cannot see what a spread passes);
// - a wrapping label or Label element;
// - an id that a label can point at: a literal id counts only when a htmlFor
//   in the same file has the same literal value, and a non-literal id counts
//   as named, since its label may live in another component.
//
// Some controls take their name another way, so only that way counts:
//
// - Slider names its thumb, the focusable part, only through thumbLabel; an
//   aria-label on Slider lands on the root.
// - cmdk labels CommandInput with a hidden label holding the enclosing
//   Command's label prop, which overrides any aria-label on the input.
// - A Button with role="combobox" is named only by aria-label or
//   aria-labelledby (or a linked id), never by the value it shows.
//
// Exempt: type="hidden", a raw input that is a button (type="submit",
// "button", "reset", or "image"), an element hidden at every width (a bare
// `hidden` class with no variant that shows it), both aria-hidden and
// tabIndex={-1} (a pointer-only duplicate of a named control), and a control
// inside a menuitemcheckbox or menuitemradio row, whose children are
// presentational (the row draws its checkbox). RadioGroup itself is not checked: group labels
// are a separate rule. See "Enforced by checks" in app/AGENTS.md, and
// app/eslint-rules.test.ts for the pinned cases and known gaps.

import {
  attribute,
  classStrings,
  elementName,
  isAriaHidden,
  isUntabbable,
} from "./control-has-name.js";

const LABELLABLE = new Set([
  "Input",
  "Textarea",
  "Checkbox",
  "Switch",
  "RadioGroupItem",
  "SelectTrigger",
  "input",
  "select",
  "textarea",
]);

const LABELS = new Set(["label", "Label"]);

// Roles whose children are presentational, so a control drawn inside one is
// not a separate control to a screen reader. Other such roles (button,
// option, tab) have no form control drawn inside them, so they stay checked.
const PRESENTATIONAL_CHILDREN = new Set(["menuitemcheckbox", "menuitemradio"]);

// A raw input of these types is a button, named by its value or alt.
const BUTTON_INPUT_TYPES = new Set(["submit", "button", "reset", "image"]);

// A bare `hidden` class hides the element at every width unless a variant
// shows it again (`hidden sm:block`, `hidden data-[state=open]:flex`).
const BARE_HIDDEN = /(^|\s)hidden(\s|$)/;
const SHOWN_SOMEWHERE =
  /(^|\s)([\w\-[\]=&>.]+:)+(block|inline|inline-block|flex|inline-flex|grid|inline-grid|table|contents)(\s|$)/;

// The string an attribute holds when it is a literal (`id="x"`, `id={"x"}`,
// or a template with no expressions), undefined when it is computed, and
// null when the attribute is absent.
const literalValue = (opening, name) => {
  const attr = attribute(opening, name);
  if (!attr) return null;
  const value = attr.value;
  if (!value) return "";
  if (value.type === "Literal") return String(value.value);
  const expression = value.expression;
  if (expression?.type === "Literal") return String(expression.value);
  if (
    expression?.type === "TemplateLiteral" &&
    expression.expressions.length === 0
  ) {
    return expression.quasis[0].value.cooked;
  }
  return undefined;
};

// Present and not the empty string.
const hasValue = (opening, name) => {
  const value = literalValue(opening, name);
  return value !== null && value !== "";
};

const hasSpread = (opening) =>
  opening.attributes.some((attr) => attr.type === "JSXSpreadAttribute");

const hasAriaName = (opening) =>
  hasValue(opening, "aria-label") || hasValue(opening, "aria-labelledby");

const isHiddenEverywhere = (opening) => {
  if (attribute(opening, "hidden")) return true;
  const classes = classStrings(opening).join(" ");
  return BARE_HIDDEN.test(classes) && !SHOWN_SOMEWHERE.test(classes);
};

// The JSX elements enclosing a node, nearest first.
function* ancestors(node) {
  for (let parent = node.parent; parent; parent = parent.parent) {
    if (parent.type === "JSXElement") yield parent.openingElement;
  }
}

const insideLabel = (node) => {
  for (const opening of ancestors(node)) {
    if (LABELS.has(elementName(opening))) return true;
  }
  return false;
};

const insidePresentationalRole = (node) => {
  for (const opening of ancestors(node)) {
    const role = literalValue(opening, "role");
    if (role && PRESENTATIONAL_CHILDREN.has(role)) return true;
  }
  return false;
};

// The control's kind, or null when the rule does not check it.
const kindOf = (opening) => {
  const name = elementName(opening);
  if (LABELLABLE.has(name)) return "labellable";
  if (name === "Slider") return "slider";
  if (name === "CommandInput") return "commandInput";
  if (name === "Button" && literalValue(opening, "role") === "combobox") {
    return "combobox";
  }
  return null;
};

const create = (context) => {
  const htmlFors = new Set();
  // Controls named only if a htmlFor in the file matches their literal id.
  const pending = [];

  return {
    JSXOpeningElement(opening) {
      const htmlFor = literalValue(opening, "htmlFor");
      if (htmlFor) htmlFors.add(htmlFor);
    },
    JSXElement(node) {
      const opening = node.openingElement;
      const kind = kindOf(opening);
      if (!kind || hasSpread(opening)) return;
      const type = literalValue(opening, "type");
      if (type === "hidden") return;
      if (elementName(opening) === "input" && BUTTON_INPUT_TYPES.has(type)) {
        return;
      }
      if (isAriaHidden(opening) && isUntabbable(opening)) return;
      if (isHiddenEverywhere(opening) || insidePresentationalRole(node)) {
        return;
      }

      if (kind === "slider") {
        if (!hasValue(opening, "thumbLabel")) {
          context.report({ node: opening, messageId: "slider" });
        }
        return;
      }

      if (kind === "commandInput") {
        for (const ancestor of ancestors(node)) {
          if (elementName(ancestor) !== "Command") continue;
          if (!hasSpread(ancestor) && !hasValue(ancestor, "label")) {
            context.report({ node: opening, messageId: "commandInput" });
          }
          return;
        }
        // No Command in this file: the label is set where Command is.
        return;
      }

      if (hasAriaName(opening)) return;
      if (kind === "labellable" && insideLabel(node)) return;
      const id = literalValue(opening, "id");
      if (id === undefined) return;
      const messageId = kind === "combobox" ? "combobox" : "unnamed";
      if (id) {
        pending.push({ id, opening, messageId });
        return;
      }
      context.report({ node: opening, messageId });
    },
    "Program:exit"() {
      for (const { id, opening, messageId } of pending) {
        if (!htmlFors.has(id)) context.report({ node: opening, messageId });
      }
    },
  };
};

export default {
  meta: {
    type: "problem",
    docs: {
      description:
        "Require every form control to have an accessible name that is not its placeholder or the value it shows.",
    },
    messages: {
      unnamed:
        "Name this form control: link its visible Label with htmlFor and a matching id (useId() when the component renders more than once), wrap it in a Label, or give it an aria-label when there is no visible label. A placeholder is not a name (see app/AGENTS.md).",
      combobox:
        "Name this combobox trigger with an aria-label or aria-labelledby that says what the field is for (its label). The value it shows is its value, not its name (see app/AGENTS.md).",
      slider:
        "Name this Slider with thumbLabel. An aria-label on Slider lands on the root, not the thumb that takes focus (see app/AGENTS.md).",
      commandInput:
        "Name this CommandInput with a label prop on its enclosing Command. cmdk labels the input from it and overrides an aria-label on the input (see app/AGENTS.md).",
    },
    schema: [],
  },
  create,
};
