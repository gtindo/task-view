// ESLint flat config enforcing the code rules in AGENTS.md on the page script.
// Core rules only, so `make lint` runs it through npx with no package.json.

export default [
  {
    files: ["internal/render/assets/app.js"],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: "script",
      globals: {
        window: "readonly",
        document: "readonly",
        location: "readonly",
        history: "readonly",
        innerWidth: "readonly",
        innerHeight: "readonly",
        localStorage: "readonly",
        URLSearchParams: "readonly",
        NodeFilter: "readonly",
        navigator: "readonly",
        console: "readonly",
        getComputedStyle: "readonly",
        requestAnimationFrame: "readonly",
        cancelAnimationFrame: "readonly",
        setTimeout: "readonly",
        clearTimeout: "readonly",
        matchMedia: "readonly",
        ResizeObserver: "readonly",
        CSS: "readonly",
        Node: "readonly",
        Map: "readonly",
        Set: "readonly",
        cytoscape: "readonly",
        cytoscapeElk: "readonly",
        ELK: "readonly",
      },
    },
    rules: {
      "no-undef": "error",
      "no-unused-vars": ["error", { argsIgnorePattern: "^_", caughtErrors: "none" }],
      // Rule 2: spacing.
      "padding-line-between-statements": [
        "error",
        { blankLine: "always", prev: "*", next: "return" },
        { blankLine: "always", prev: ["const", "let"], next: "*" },
        { blankLine: "any", prev: ["const", "let"], next: ["const", "let"] },
      ],
      // Rule 3: guard clauses.
      "no-else-return": ["error", { allowElseIf: false }],
      "max-depth": ["error", 3],
      // Rule 4: names.
      "id-length": ["error", { min: 2, exceptions: ["i", "j", "k", "_"], properties: "never" }],
      // Rule 5: magic values.
      "no-magic-numbers": [
        "error",
        { ignore: [0, 1, -1, 2, 100], ignoreArrayIndexes: true, ignoreDefaultValues: true },
      ],
      // Rule 10: functional style.
      "prefer-const": "error",
      "no-var": "error",
      "no-param-reassign": ["error", { props: true }],
    },
  },
];
