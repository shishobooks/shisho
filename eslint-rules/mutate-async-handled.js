// A mutateAsync promise rejects when the request fails, so one that nothing
// catches is an unhandled rejection with no report to the user. This rule
// accepts a mutateAsync call only when, within its own function:
//
// - it is awaited inside the block of a try whose catch reports something
//   (a try does not see a promise it does not await, and an empty catch, or
//   one that only logs to the console and rethrows, reports nothing), or
// - its chain ends in .catch(handler) or .then(onOk, onError), or
// - its promise is returned (return or a concise arrow body) from a function
//   whose caller may catch it. That is any function except one written
//   inline in JSX (also as a ternary branch or behind &&), one bound to a
//   variable that a JSX event prop references (directly or through
//   useCallback), and one passed to a call whose own
//   result goes unhandled (the promise flows out of that call, so
//   Promise.all(ids.map((id) => m.mutateAsync(id))) needs its own catch).
//
// A component callback prop such as onSave is not an event prop: the
// metadata detail pages return their promises to the edit, merge, and delete
// dialogs, which catch them. Destructured, aliased, computed, and uncalled
// mutateAsync references are banned by no-restricted-syntax selectors in
// eslint.config.js, so this rule only sees calls on the mutation object. See
// "Every mutate() passes onError" in app/AGENTS.md, and
// app/eslint-rules.test.ts for the pinned cases and known gaps.

const FUNCTIONS = new Set([
  "ArrowFunctionExpression",
  "FunctionDeclaration",
  "FunctionExpression",
]);

// Expression wrappers the promise passes through unchanged.
const WRAPPERS = new Set([
  "ChainExpression",
  "TSAsExpression",
  "TSNonNullExpression",
  "TSSatisfiesExpression",
]);

// Props whose handler is called by the DOM or a UI primitive, which drops
// the returned promise. Component callback props (onSave, onDelete) are not
// listed: their component may catch. The list is best effort, covering the
// DOM and Radix events the app uses; add a name when a new one appears.
const EVENT_PROP =
  /^(on(Click|DoubleClick|ContextMenu|Submit|Change|Input|Key(Down|Up|Press)|EscapeKeyDown|Blur|Focus|(Mouse|Pointer|Touch|Drag)[A-Z]\w*|Drop|Select|Paste|Copy|Cut|Scroll|Wheel|Load|Error|CheckedChange|ValueChange|ValueCommit|OpenChange|PressedChange|InteractOutside|(Open|Close)AutoFocus)|action|formAction)$/;

const unwrap = (node) => {
  while (WRAPPERS.has(node.parent?.type)) node = node.parent;
  return node;
};

const isMutateAsyncCall = (node) =>
  node.callee.type === "MemberExpression" &&
  !node.callee.computed &&
  node.callee.property.type === "Identifier" &&
  node.callee.property.name === "mutateAsync";

const isUseCallback = (callee) =>
  (callee.type === "Identifier" && callee.name === "useCallback") ||
  (callee.type === "MemberExpression" &&
    !callee.computed &&
    callee.property.name === "useCallback");

const isConsoleCall = (statement) =>
  statement.type === "ExpressionStatement" &&
  statement.expression.type === "CallExpression" &&
  statement.expression.callee.type === "MemberExpression" &&
  statement.expression.callee.object.type === "Identifier" &&
  statement.expression.callee.object.name === "console";

// A catch reports the failure unless it is empty or holds only console calls
// and throws.
const catchReports = (handler) =>
  handler.body.body.some(
    (statement) =>
      statement.type !== "ThrowStatement" && !isConsoleCall(statement),
  );

// Whether the promise is awaited inside the block of a try that reports.
const inReportingTry = (node) => {
  let awaited = false;
  for (
    let child = node, parent = node.parent;
    parent && !FUNCTIONS.has(parent.type);
    child = parent, parent = parent.parent
  ) {
    if (parent.type === "AwaitExpression") awaited = true;
    if (
      awaited &&
      parent.type === "TryStatement" &&
      parent.block === child &&
      parent.handler &&
      catchReports(parent.handler)
    ) {
      return true;
    }
  }
  return false;
};

// The outermost expression a function value reaches unchanged: through
// wrappers, a ternary branch, or either side of && and ||.
const valueRoot = (fn) => {
  let node = fn;
  for (;;) {
    const parent = node.parent;
    if (
      WRAPPERS.has(parent.type) ||
      parent.type === "LogicalExpression" ||
      (parent.type === "ConditionalExpression" && parent.test !== node)
    ) {
      node = parent;
    } else {
      return node;
    }
  }
};

const enclosingFunction = (node) => {
  let parent = node.parent;
  while (parent && !FUNCTIONS.has(parent.type)) parent = parent.parent;
  return parent;
};

const isEventPropValue = (identifier) =>
  identifier.parent.type === "JSXExpressionContainer" &&
  identifier.parent.parent.type === "JSXAttribute" &&
  typeof identifier.parent.parent.name.name === "string" &&
  EVENT_PROP.test(identifier.parent.parent.name.name);

const create = (context) => {
  const { sourceCode } = context;

  const referencedFromEventProp = (declaration) =>
    sourceCode
      .getDeclaredVariables(declaration)
      .some((variable) =>
        variable.references.some((reference) =>
          isEventPropValue(reference.identifier),
        ),
      );

  // Whether a function's caller receives the promise it returns and may
  // catch it.
  const isHandOff = (fn) => {
    if (fn.type === "FunctionDeclaration") return !referencedFromEventProp(fn);
    const value = valueRoot(fn);
    const parent = value.parent;
    if (parent.type === "JSXExpressionContainer") return false;
    if (parent.type === "VariableDeclarator" && parent.init === value) {
      return !referencedFromEventProp(parent);
    }
    if (parent.type === "CallExpression" && parent.arguments.includes(value)) {
      if (!isUseCallback(parent.callee)) return isHandled(parent);
      const declarator = unwrap(parent).parent;
      return (
        declarator.type === "VariableDeclarator" &&
        !referencedFromEventProp(declarator)
      );
    }
    return true;
  };

  // Whether the promise an expression evaluates to is caught.
  const isHandled = (expression) => {
    let node = unwrap(expression);
    for (;;) {
      const member = node.parent;
      const call = member.parent;
      if (
        member.type !== "MemberExpression" ||
        member.object !== node ||
        member.computed ||
        call?.type !== "CallExpression" ||
        call.callee !== member
      ) {
        break;
      }
      const method = member.property.name;
      if (method === "catch" && call.arguments.length > 0) return true;
      if (method === "then" && call.arguments.length > 1) return true;
      if (method !== "then" && method !== "finally") break;
      node = unwrap(call);
    }
    if (inReportingTry(node)) return true;
    const parent = node.parent;
    const returned =
      parent.type === "ReturnStatement" ||
      (parent.type === "ArrowFunctionExpression" && parent.body === node);
    return returned && isHandOff(enclosingFunction(node));
  };

  return {
    CallExpression(node) {
      if (isMutateAsyncCall(node) && !isHandled(node)) {
        context.report({ node, messageId: "unhandled" });
      }
    },
  };
};

export default {
  meta: {
    type: "problem",
    docs: {
      description:
        "Require every mutateAsync promise to be caught and the failure reported.",
    },
    messages: {
      unhandled:
        "Await mutateAsync in a try/catch that reports the failure with toastRequestError, chain .catch(), or return its promise to a caller that catches it (not an event handler).",
    },
    schema: [],
  },
  create,
};
