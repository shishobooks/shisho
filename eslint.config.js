import js from "@eslint/js";
import reactPlugin from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

import controlHasName from "./eslint-rules/control-has-name.js";
import formControlHasName from "./eslint-rules/form-control-has-name.js";
import mutateAsyncHandled from "./eslint-rules/mutate-async-handled.js";

// Permission checks take a typed requirement through useCan or can (see
// "Permissions" in app/AGENTS.md), so a typo fails to compile.
// hasPermission takes plain strings, where a typo fails silently.
const literalPermissionCheck = {
  selector:
    'CallExpression:matches([callee.name="hasPermission"], [callee.property.name="hasPermission"]):matches([arguments.0.type=/^(Literal|TemplateLiteral)$/], [arguments.1.type=/^(Literal|TemplateLiteral)$/])',
  message:
    'Use useCan("resource:operation") or can(...) from useAuth(), which type-check the permission.',
};

// The imperative forms of useQuery. A query started this way skips the
// permission gate a query hook in app/hooks/queries applies.
const imperativeQueries = {
  selector:
    "CallExpression[callee.property.name=/^(fetchQuery|prefetchQuery|ensureQueryData|fetchInfiniteQuery|prefetchInfiniteQuery|ensureInfiniteQueryData)$/]",
  message:
    "Fetch through a query hook in app/hooks/queries that gates on its route's permission (see app/AGENTS.md).",
};

// Cover and page endpoints are served immutable, so their URLs must carry the
// cache key the helpers in app/utils add; download and stream URLs are built
// there too so each path is written once. A literal /api/... URL ending in
// one of these segments anywhere else is a hand-built copy. esquery regexes
// cannot contain a slash, so the patterns spell it \x2F.
const API_PREFIX = String.raw`^\x2Fapi\x2F`;
const FILE_SEGMENT = String.raw`\x2F(cover|download|stream|page)([?\x2F]|$)`;
const FILE_URL_MESSAGE =
  "Build cover, page, download and stream URLs with the helpers in app/utils (coverUrl.ts, pageUrl.ts, downloadUrl.ts).";
const literalFileUrls = [
  {
    selector: `TemplateLiteral:has(TemplateElement[value.raw=/${API_PREFIX}/]):has(TemplateElement[value.raw=/${FILE_SEGMENT}/])`,
    message: FILE_URL_MESSAGE,
  },
  {
    selector: `Literal[value=/${API_PREFIX}.*${FILE_SEGMENT}/]`,
    message: FILE_URL_MESSAGE,
  },
  {
    // "/api/books/" + id + "/cover", reported once on the outermost `+`.
    selector: `BinaryExpression[operator="+"]:not(BinaryExpression > BinaryExpression):has(Literal[value=/${API_PREFIX}/]):has(Literal[value=/${FILE_SEGMENT}/])`,
    message: FILE_URL_MESSAGE,
  },
];

// Mechanical UI conventions from app/AGENTS.md. app/components/ui is ignored
// above, so the shadcn primitives are exempt.
const uiConventions = [
  {
    // A template literal skips tailwind-merge's conflict resolution.
    selector:
      'JSXAttribute[name.name="className"] > JSXExpressionContainer > TemplateLiteral',
    message:
      "Compose className with cn() from @/libraries/utils, not a template literal (see app/AGENTS.md).",
  },
  ...[
    "TemplateElement[value.raw=/\\sago\\b/]",
    "Literal[value=/\\sago\\b/]",
    "JSXText[value=/\\sago\\b/]",
  ].map((selector) => ({
    selector,
    message:
      'Write relative times with formatDistanceToNow(date, { addSuffix: true }) from date-fns, not a hand-appended "ago" (see app/AGENTS.md).',
  })),
  ...[
    'BinaryExpression[operator=/^[!=]==?$/][left.property.name="status"][right.value=404]',
    'BinaryExpression[operator=/^[!=]==?$/][right.property.name="status"][left.value=404]',
  ].map((selector) => ({
    selector,
    message:
      "Use isNotFoundError(error) or isLoadFailure(query) from @/libraries/api, not an inline status check for 404 (see app/AGENTS.md).",
  })),
  {
    // A raw button skips the shared focus ring, cursor, disabled state, and
    // type="button" default. unstyled is the escape hatch for rows, cards,
    // and tiles the caller styles itself.
    selector: 'JSXOpeningElement[name.name="button"]',
    message:
      'Use Button from @/components/ui/button, not a raw <button>: size="icon", "icon-sm", or "icon-xs" for icon-only buttons (with an aria-label), variant="link" for inline text, variant="unstyled" for caller-styled rows, cards, and tiles (with aria-pressed inside a role="group" for segmented controls and single-select rows), BadgeRemoveButton for a Badge\'s X, or the ui Checkbox or RadioGroup for form toggles.',
  },
  {
    selector:
      'JSXOpeningElement[name.name="Tabs"] > JSXAttribute[name.name="defaultValue"]',
    message:
      'Deep-link tabs: pass value and onValueChange synced to the URL instead of defaultValue (see "Every page" in docs/agents/standards/frontend.md).',
  },
];

// A mutation fired with mutate() reports a rejection only through onError, so
// one without it fails silently. Toast it with toastRequestError (see
// "Request errors" in app/AGENTS.md), or use mutateAsync in a
// try/catch. The options must be an object literal passed straight to
// mutate(), so an onError inside a nested call does not count, options held
// in a variable are flagged because the rule cannot see into them, and
// `onError: undefined` counts as missing.
const MUTATE_MESSAGE =
  "Pass onError to mutate() and report the failure with toastRequestError, or await mutateAsync in a try/catch.";
const MUTATE_CALL = 'CallExpression[callee.property.name="mutate"]';
const mutateWithoutOnError = [
  // No options at all.
  { selector: `${MUTATE_CALL}[arguments.length<2]`, message: MUTATE_MESSAGE },
  // Options without their own onError (one in a nested call does not count).
  {
    selector: `${MUTATE_CALL} > ObjectExpression:nth-child(2):not(:has(> Property[key.name="onError"]:not([value.type="Identifier"][value.name="undefined"])))`,
    message: MUTATE_MESSAGE,
  },
  // Options the rule cannot see into.
  {
    selector: `${MUTATE_CALL} > .arguments:nth-child(2):not(ObjectExpression)`,
    message: MUTATE_MESSAGE,
  },
];

// The mutate and mutateAsync checks read calls on the mutation object
// (`m.mutate(...)`), so a destructured, renamed, aliased, computed, or
// uncalled reference would slip past them. Ban those shapes outright. An
// uncalled `onClick={m.mutate}` would also pass the click event as the
// mutation's variables.
const mutationReferences = [
  ["mutate", "Call mutate on the mutation object and pass onError"],
  [
    "mutateAsync",
    "Call mutateAsync on the mutation object and catch its promise",
  ],
].flatMap(([name, action]) => {
  const message = `${action}: a destructured, aliased, computed, or uncalled ${name} hides the call from the lint rule that checks it.`;
  return [
    {
      selector: `ObjectPattern > Property:matches([key.name="${name}"], [key.value="${name}"])`,
      message,
    },
    {
      selector: `MemberExpression[property.name="${name}"]:not(CallExpression > MemberExpression.callee)`,
      message,
    },
    {
      selector: `MemberExpression[computed=true][property.value="${name}"]`,
      message,
    },
  ];
});

// Every mutateAsync call on a mutation object must be caught. esquery cannot
// express "within the nearest enclosing function", so this is a local rule;
// eslint-rules/mutate-async-handled.js explains what it accepts. Every
// button and link needs an accessible name at every width, which takes
// following ternaries and breakpoint classes through its children;
// eslint-rules/control-has-name.js explains what counts as a name. Every
// form control needs one too, which takes matching each id to a htmlFor
// across the file; eslint-rules/form-control-has-name.js explains how.
const shishoPlugin = {
  rules: {
    "control-has-name": controlHasName,
    "form-control-has-name": formControlHasName,
    "mutate-async-handled": mutateAsyncHandled,
  },
};

export default tseslint.config(
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    ignores: [
      "build/*",
      "pkg/frontend/dist/**",
      "website/build/**",
      "website/.docusaurus/**",
      "app/types/generated/*",
      "app/components/ui",
      "app/libraries/foliate/**",
      "test-results",
      "pkg/plugins/testdata/*",
      "packages/plugin-sdk/testing/index.js",
      "packages/plugin-sdk/testing/index.d.ts",
      "tmp",
    ],
  },
  {
    files: ["app/**/*.{ts,tsx}", "website/src/**/*.{ts,tsx}"],
    ...reactPlugin.configs.flat["jsx-runtime"],
    languageOptions: {
      ...reactPlugin.configs.flat["jsx-runtime"].languageOptions,
      ecmaVersion: 2020,
      globals: {
        ...globals.browser,
        __APP_VERSION__: "readonly",
      },
    },
    plugins: {
      ...reactPlugin.configs.flat["jsx-runtime"].plugins,
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      // Disable the new v7 rule - our patterns are intentional (syncing state when props change)
      "react-hooks/set-state-in-effect": "off",
      "react-refresh/only-export-components": [
        "warn",
        { allowConstantExport: true },
      ],
      "@typescript-eslint/no-non-null-assertion": ["off"],
      "react/jsx-sort-props": ["error"],
    },
  },
  {
    // Queries live in app/hooks/queries, where each hook gates itself on its
    // route's permission and hooks/queries/permissions.test.tsx checks it. A
    // query written in a component would skip both. Tests may build queries.
    files: ["app/**/*.{ts,tsx}"],
    ignores: ["app/hooks/queries/**", "app/**/*.test.{ts,tsx}"],
    plugins: { shisho: shishoPlugin },
    rules: {
      "no-restricted-syntax": [
        "error",
        literalPermissionCheck,
        imperativeQueries,
        ...mutateWithoutOnError,
        ...mutationReferences,
        ...literalFileUrls,
        ...uiConventions,
      ],
      "shisho/mutate-async-handled": "error",
      "shisho/control-has-name": "error",
      "shisho/form-control-has-name": "error",
      "no-restricted-properties": [
        "error",
        {
          object: "navigator",
          property: "clipboard",
          message:
            "Copy with copyText from @/utils/clipboard, which falls back when navigator.clipboard is undefined over plain HTTP (see app/AGENTS.md).",
        },
      ],
      "no-restricted-imports": [
        "error",
        {
          paths: [
            {
              name: "@tanstack/react-query",
              importNames: [
                "useQuery",
                "useQueries",
                "useInfiniteQuery",
                "useSuspenseQuery",
                "useSuspenseQueries",
                "useSuspenseInfiniteQuery",
              ],
              message:
                "Add a query hook in app/hooks/queries that gates on its route's permission (see app/AGENTS.md).",
            },
          ],
        },
      ],
    },
  },
  {
    // The block above skips app/hooks/queries for the query rules, and a later
    // no-restricted-syntax setting would replace its selectors, so the query
    // hooks list the rules that still apply to them.
    files: ["app/hooks/queries/**/*.{ts,tsx}"],
    ignores: ["app/**/*.test.{ts,tsx}"],
    rules: {
      "no-restricted-syntax": [
        "error",
        literalPermissionCheck,
        ...literalFileUrls,
        ...uiConventions,
      ],
    },
  },
  {
    // ResourceListResponse is the structural prop type for generic list
    // components. A hook typed with it would not notice a change to the Go
    // envelope, so hooks use the generated List*Response (ADR 0004).
    files: ["app/hooks/queries/**/*.{ts,tsx}"],
    ignores: ["app/**/*.test.{ts,tsx}"],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          paths: ["@/types", "@/types/index"].map((name) => ({
            name,
            importNames: ["ResourceListResponse"],
            message:
              "Type the hook with the generated List*Response for its endpoint, so a Go response change fails the typecheck (see app/AGENTS.md).",
          })),
        },
      ],
    },
  },
  {
    // app/utils holds the URL helpers, so it skips only the URL rule. This
    // replaces the app block's no-restricted-syntax list and keeps its
    // no-restricted-imports query ban.
    files: ["app/utils/**/*.{ts,tsx}"],
    ignores: ["app/**/*.test.{ts,tsx}"],
    rules: {
      "no-restricted-syntax": [
        "error",
        literalPermissionCheck,
        imperativeQueries,
        ...mutateWithoutOnError,
        ...mutationReferences,
        ...uiConventions,
      ],
    },
  },
  {
    // copyText is the one place allowed to touch the async clipboard.
    files: ["app/utils/clipboard.ts"],
    rules: { "no-restricted-properties": "off" },
  },
  {
    // e2e/fixtures.ts extends Playwright's test with fixtures that point each
    // browser project at its own API server; importing test from
    // @playwright/test directly skips them. Keeping every value import in
    // fixtures.ts leaves one place to add shared setup.
    files: ["e2e/**/*.ts"],
    ignores: ["e2e/fixtures.ts"],
    rules: {
      "@typescript-eslint/no-restricted-imports": [
        "error",
        {
          paths: [
            {
              name: "@playwright/test",
              allowTypeImports: true,
              message:
                "Import test, expect, and request from ./fixtures, which point each browser at its own API server (see e2e/AGENTS.md). Type-only imports are fine.",
            },
          ],
        },
      ],
    },
  },
  {
    files: ["*.js", "eslint-rules/*.js"],
    ignores: ["app/**", "website/**"],
    languageOptions: {
      globals: globals.node,
    },
  },
  {
    files: ["website/*.{js,ts}"],
    languageOptions: {
      globals: globals.node,
    },
  },
  {
    files: ["packages/**/*.ts"],
    languageOptions: {
      globals: globals.node,
    },
  },
  {
    rules: {
      "comma-dangle": ["error", "always-multiline"],
    },
  },
);
