import js from "@eslint/js";
import reactPlugin from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

// Permission checks take a typed requirement through useCan or can (see
// "Permission-gated controls" in app/AGENTS.md), so a typo fails to compile.
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

// A mutation fired with mutate() reports a rejection only through onError, so
// one without it fails silently. Toast it with toastRequestError (see
// "Request errors and retries" in app/AGENTS.md), or use mutateAsync in a
// try/catch. The options must be an object literal passed straight to
// mutate(), so an onError inside a nested call does not count, and options
// held in a variable are flagged because the rule cannot see into them.
const MUTATE_MESSAGE =
  "Pass onError to mutate() and report the failure with toastRequestError, or await mutateAsync in a try/catch.";
const MUTATE_CALL = 'CallExpression[callee.property.name="mutate"]';
const mutateWithoutOnError = [
  // No options at all.
  { selector: `${MUTATE_CALL}[arguments.length<2]`, message: MUTATE_MESSAGE },
  // Options without their own onError (one in a nested call does not count).
  {
    selector: `${MUTATE_CALL} > ObjectExpression:nth-child(2):not(:has(> Property[key.name="onError"]))`,
    message: MUTATE_MESSAGE,
  },
  // Options the rule cannot see into.
  {
    selector: `${MUTATE_CALL} > .arguments:nth-child(2):not(ObjectExpression)`,
    message: MUTATE_MESSAGE,
  },
];

// A mutateAsync promise rejects when the request fails, so one that nothing
// catches is an unhandled rejection with no report to the user. The call must
// sit in the block of a try that has a catch clause (not in the catch or
// finally), in a chain that ends in .catch(), or hand its promise to the
// caller with return or a concise arrow body, which makes the caller
// responsible (MetadataDeleteDialog catches the promise the detail pages'
// onDelete returns). A concise arrow in a JSX event prop is still flagged: an
// event handler never catches what it is handed. The rule reads syntax only,
// so it cannot tell whether a caller really catches a returned promise, or
// whether a try catches a call made later from a callback defined inside it.
const MUTATE_ASYNC_MESSAGE =
  "Await mutateAsync in a try/catch that reports the failure with toastRequestError, chain .catch(), or return its promise to a caller that catches it.";
const MUTATE_ASYNC_CALL = 'CallExpression[callee.property.name="mutateAsync"]';
const mutateAsyncUnhandled = [
  {
    selector: [
      MUTATE_ASYNC_CALL,
      ":not(TryStatement[handler] > BlockStatement.block CallExpression)",
      ':not(CallExpression[callee.property.name="catch"] > MemberExpression.callee CallExpression)',
      ":not(ReturnStatement > CallExpression)",
      ":not(ArrowFunctionExpression > CallExpression.body)",
    ].join(""),
    message: MUTATE_ASYNC_MESSAGE,
  },
  {
    selector: `JSXAttribute[name.name=/^on[A-Z]/] > JSXExpressionContainer > ArrowFunctionExpression > ${MUTATE_ASYNC_CALL}.body`,
    message: MUTATE_ASYNC_MESSAGE,
  },
];

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
    rules: {
      "no-restricted-syntax": [
        "error",
        literalPermissionCheck,
        imperativeQueries,
        ...mutateWithoutOnError,
        ...mutateAsyncUnhandled,
        ...literalFileUrls,
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
        ...mutateAsyncUnhandled,
      ],
    },
  },
  {
    files: ["*.js"],
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
