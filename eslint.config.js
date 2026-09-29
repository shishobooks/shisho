import js from "@eslint/js";
import reactPlugin from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

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
