import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import eslintConfigPrettier from "eslint-config-prettier";

import {
  PHYSICAL_CLASS_IGNORES,
  findPhysicalClasses,
} from "./scripts/physical-classes.mjs";

/**
 * local/no-physical-classes (TEC-137, K10): RTL needs logical utilities.
 * Reports ml-/pr-/left-/border-l/rounded-r/text-left... in string and
 * template literals; `pnpm codemod:logical` rewrites them.
 */
const noPhysicalClasses = {
  meta: {
    type: "problem",
    messages: {
      physical:
        'Physical utility "{{token}}" breaks RTL; use "{{logical}}" (pnpm codemod:logical).',
    },
    schema: [],
  },
  create(context) {
    const check = (node, raw) => {
      for (const hit of findPhysicalClasses(raw)) {
        context.report({
          node,
          messageId: "physical",
          data: { token: hit.token, logical: hit.logical },
        });
      }
    };
    return {
      Literal(node) {
        if (typeof node.value === "string") check(node, node.value);
      },
      TemplateElement(node) {
        check(node, node.value.raw);
      },
    };
  },
};

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  eslintConfigPrettier,
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/components/ui/date-picker.tsx"],
    rules: {
      "@typescript-eslint/no-unused-vars": [
        "warn",
        {
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
          caughtErrorsIgnorePattern: "^_",
          ignoreRestSiblings: true,
        },
      ],
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='type'][value.value='date']",
          message:
            'Use DatePicker from @/components/ui/date-picker (or AppDatePicker in forms) instead of native type="date".',
        },
      ],
    },
  },
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: PHYSICAL_CLASS_IGNORES.map((dir) => `${dir}**`),
    plugins: { local: { rules: { "no-physical-classes": noPhysicalClasses } } },
    rules: { "local/no-physical-classes": "error" },
  },
  // Editor X (Lexical / shadcn-editor) uses ref-during-render and effect
  // patterns that conflict with React Compiler eslint rules.
  {
    files: ["src/components/editor/**/*.{ts,tsx}"],
    rules: {
      "react-hooks/refs": "off",
      "react-hooks/set-state-in-effect": "off",
      "react-hooks/use-memo": "off",
      "react/display-name": "off",
      "@next/next/no-img-element": "off",
      "@typescript-eslint/no-empty-object-type": "off",
      "@typescript-eslint/ban-ts-comment": "off",
    },
  },
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    "src/generated/**",
  ]),
]);

export default eslintConfig;
