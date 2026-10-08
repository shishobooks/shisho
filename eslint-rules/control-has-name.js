// A screen reader announces a button or link by its accessible name, which
// comes from an aria-label, aria-labelledby, or the text inside it. An
// icon-only control has none, and neither does a header button whose label
// sits in a `hidden sm:inline` span, since that span is display:none on a
// phone. A title attribute is a tooltip, not a reliable name, so it does not
// count. This rule flags Button, BadgeRemoveButton, Link, and <a> elements
// that have:
//
// - no aria-label or aria-labelledby, no spread props (the rule cannot see
//   what a spread passes), and not both aria-hidden and tabIndex={-1} (a
//   pointer-only duplicate of a named control), and
// - no text that renders at every width: a JSX text node, a string, or an
//   expression that may hold one (a variable, a call), outside any element
//   that is aria-hidden or hidden at some width (`hidden sm:inline`,
//   `sm:hidden`, `invisible`). An `sr-only` element and a non-empty alt
//   count as text.
//
// Both branches of a ternary must hold text, since either may render, and
// `cond && "text"` holds none, since it renders nothing when cond is false.
// A Button with asChild lends its props to the element it renders as (also
// either branch of a ternary child), so the pair is checked as one element. A
// variable named for an icon (`item.icon`, `StatusIcon`) is not text. See
// "Enforced by checks" in app/AGENTS.md, and app/eslint-rules.test.ts for the
// pinned cases and known gaps.

const CONTROLS = new Set(["Button", "BadgeRemoveButton", "Link", "a"]);

const NAME_ATTRIBUTES = new Set(["aria-label", "aria-labelledby"]);

// `hidden` or `invisible`, alone or behind variants (`sm:hidden`), hides the
// element at some width or state, so its text is not a name there.
// `overflow-hidden` does not match.
const HIDDEN_SOMEWHERE = /(^|\s)([\w-]+:)*(hidden|invisible)(\s|$)/;
const SR_ONLY = /(^|\s)sr-only(\s|$)/;

export const elementName = (opening) =>
  opening.name.type === "JSXIdentifier" ? opening.name.name : null;

export const attribute = (opening, name) =>
  opening.attributes.find(
    (attr) => attr.type === "JSXAttribute" && attr.name.name === name,
  );

// The literal class strings of a className, including those passed to cn().
export const classStrings = (opening) => {
  const attr = attribute(opening, "className");
  if (!attr?.value) return [];
  if (attr.value.type === "Literal") return [String(attr.value.value)];
  const strings = [];
  const visit = (node) => {
    if (!node || typeof node !== "object") return;
    if (node.type === "Literal" && typeof node.value === "string") {
      strings.push(node.value);
    } else if (node.type === "TemplateElement") {
      strings.push(node.value.raw);
    }
    for (const key of Object.keys(node)) {
      if (key === "parent") continue;
      const child = node[key];
      if (Array.isArray(child)) child.forEach(visit);
      else if (child && typeof child.type === "string") visit(child);
    }
  };
  visit(attr.value);
  return strings;
};

const hasClass = (opening, pattern) =>
  classStrings(opening).some((value) => pattern.test(value));

export const isAriaHidden = (opening) => {
  const attr = attribute(opening, "aria-hidden");
  if (!attr) return false;
  if (!attr.value) return true;
  if (attr.value.type === "Literal") return attr.value.value !== "false";
  const expression = attr.value.expression;
  return !(expression?.type === "Literal" && expression.value === false);
};

// The string an attribute holds when it is a literal (`id="x"`, `id={"x"}`,
// or a template with no expressions), undefined when it is computed, and
// null when the attribute is absent.
export const literalValue = (opening, name) => {
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
export const hasValue = (opening, name) => {
  const value = literalValue(opening, name);
  return value !== null && value !== "";
};

export const hasSpread = (opening) =>
  opening.attributes.some((attr) => attr.type === "JSXSpreadAttribute");

export const hasAriaName = (opening) =>
  hasValue(opening, "aria-label") || hasValue(opening, "aria-labelledby");

// An attribute set to `{undefined}`, which React drops, so it is absent.
export const isUndefinedValue = (opening, name) => {
  const expression = attribute(opening, name)?.value?.expression;
  return expression?.type === "Identifier" && expression.name === "undefined";
};

// tabIndex={-1} or tabIndex="-1".
export const isUntabbable = (opening) => {
  const value = attribute(opening, "tabIndex")?.value;
  if (value?.type === "Literal") return value.value === "-1";
  const expression = value?.expression;
  return (
    (expression?.type === "UnaryExpression" &&
      expression.operator === "-" &&
      expression.argument.type === "Literal" &&
      expression.argument.value === 1) ||
    (expression?.type === "Literal" && expression.value === -1)
  );
};

// Named by its attributes, or out of both the accessibility tree and the tab
// order, so it needs no name.
const isNamed = (opening) =>
  (isAriaHidden(opening) && isUntabbable(opening)) ||
  opening.attributes.some(
    (attr) =>
      attr.type === "JSXSpreadAttribute" ||
      (NAME_ATTRIBUTES.has(attr.name.name) &&
        !(attr.value?.type === "Literal" && attr.value.value === "")),
  );

// A variable or property named for an icon (`icon`, `item.icon`, `StatusIcon`)
// holds an element, not text. `lexicon` is not one.
const ICON_NAME = /(^icon|Icon)$/;

// Whether an expression in a JSX child position renders text in every case
// the rule can see. Anything it cannot follow (a variable, a call, a map)
// is assumed to hold text, unless its name says it is an icon.
const expressionHasText = (node, slot = false) => {
  switch (node.type) {
    case "JSXEmptyExpression":
      return false;
    case "Literal":
      return (
        (typeof node.value === "string" && node.value.trim() !== "") ||
        typeof node.value === "number"
      );
    case "TemplateLiteral":
      return (
        node.expressions.length > 0 ||
        node.quasis.some((quasi) => quasi.value.raw.trim() !== "")
      );
    case "JSXElement":
      return elementHasText(node, slot);
    case "JSXFragment":
      return childrenHaveText(node.children);
    case "ConditionalExpression":
      return (
        expressionHasText(node.consequent, slot) &&
        expressionHasText(node.alternate, slot)
      );
    case "LogicalExpression":
      if (node.operator === "&&") return false;
      return (
        expressionHasText(node.left, slot) ||
        expressionHasText(node.right, slot)
      );
    case "TSAsExpression":
    case "TSNonNullExpression":
    case "TSSatisfiesExpression":
      return expressionHasText(node.expression, slot);
    case "Identifier":
      return !ICON_NAME.test(node.name);
    case "MemberExpression":
      return node.computed || !ICON_NAME.test(node.property.name);
    default:
      return true;
  }
};

// `slot` marks the children of an asChild Button, where the element that
// renders in the Button's place may carry the name itself.
const childrenHaveText = (children, slot = false) =>
  children.some((child) => {
    if (child.type === "JSXText") return child.value.trim() !== "";
    if (child.type === "JSXExpressionContainer") {
      return expressionHasText(child.expression, slot);
    }
    if (child.type === "JSXSpreadChild") return true;
    return expressionHasText(child, slot);
  });

// Whether an element nested in a control gives it a name.
const elementHasText = (element, slot = false) => {
  const opening = element.openingElement;
  if (slot && isNamed(opening)) return true;
  if (hasClass(opening, SR_ONLY)) return true;
  if (isAriaHidden(opening) || hasClass(opening, HIDDEN_SOMEWHERE)) {
    return false;
  }
  const alt = attribute(opening, "alt");
  if (alt && !(alt.value?.type === "Literal" && alt.value.value === "")) {
    return true;
  }
  return childrenHaveText(element.children);
};

const isAsChild = (opening) => {
  const attr = attribute(opening, "asChild");
  if (!attr) return false;
  if (!attr.value) return true;
  const expression = attr.value.expression;
  return !(expression?.type === "Literal" && expression.value === false);
};

// Expression nodes a slot child can sit in: `{cond ? <a /> : <span />}`.
const SLOT_PATHS = new Set([
  "JSXExpressionContainer",
  "ConditionalExpression",
  "LogicalExpression",
]);

// The asChild Button an element renders as, if any. Slot merges the
// Button's props onto that element, so the Button checks the pair.
const slotOwner = (node) => {
  let parent = node.parent;
  while (parent && SLOT_PATHS.has(parent.type)) parent = parent.parent;
  if (
    parent?.type === "JSXElement" &&
    elementName(parent.openingElement) === "Button" &&
    isAsChild(parent.openingElement)
  ) {
    return parent;
  }
  return null;
};

const create = (context) => ({
  JSXElement(node) {
    const opening = node.openingElement;
    const name = elementName(opening);
    if (!CONTROLS.has(name) || slotOwner(node)) return;
    if (isNamed(opening)) return;
    const slot = name === "Button" && isAsChild(opening);
    if (childrenHaveText(node.children, slot)) return;
    context.report({ node: opening, messageId: "unnamed" });
  },
});

export default {
  meta: {
    type: "problem",
    docs: {
      description:
        "Require every Button, BadgeRemoveButton, Link, and <a> to have an accessible name at every width.",
    },
    messages: {
      unnamed:
        'Give this control an accessible name at every width: an aria-label that says what it does ("Remove path", or the visible text when that text is hidden on phones), aria-labelledby, or text that is not inside a hidden or aria-hidden element. A title is not a name (see app/AGENTS.md).',
    },
    schema: [],
  },
  create,
};
