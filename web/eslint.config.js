import js from "@eslint/js"
import jsxA11y from "eslint-plugin-jsx-a11y"
import reactHooks from "eslint-plugin-react-hooks"
import reactRefresh from "eslint-plugin-react-refresh"
import globals from "globals"
import tseslint from "typescript-eslint"

export default tseslint.config(
  { ignores: ["dist"] },
  {
    files: ["**/*.{ts,tsx}"],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommendedTypeChecked,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
      jsxA11y.flatConfigs.recommended,
    ],
    languageOptions: {
      globals: globals.browser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      // shadcn's components export their variants and hooks beside the components.
      "react-refresh/only-export-components": [
        "error",
        {
          allowConstantExport: true,
          allowCompoundComponents: true,
          allowExportNames: [
            "buttonVariants",
            "markerVariants",
            "useSidebar",
            "useMessageScroller",
            "useMessageScrollerScrollable",
            "useMessageScrollerVisibility",
          ],
        },
      ],
    },
  },
  {
    files: ["**/*.js"],
    extends: [js.configs.recommended],
    languageOptions: { globals: globals.node },
  },
)
