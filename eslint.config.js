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
    ignores: ["build/*", "app/types/generated/*", "app/components/ui"],
  },
  {
    files: ["app/**/*.{ts,tsx}"],
    ...reactPlugin.configs.flat["jsx-runtime"],
    languageOptions: {
      ...reactPlugin.configs.flat["jsx-runtime"].languageOptions,
      ecmaVersion: 2020,
      globals: globals.browser,
    },
    plugins: {
      ...reactPlugin.configs.flat["jsx-runtime"].plugins,
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": [
        "warn",
        { allowConstantExport: true },
      ],
      "@typescript-eslint/no-non-null-assertion": ["off"],
      "react/jsx-sort-props": ["error"],
    },
  },
  {
    files: ["*.js"],
    ignores: ["app/**"],
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
