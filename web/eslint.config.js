import js from '@eslint/js'
import i18next from 'eslint-plugin-i18next'
import reactHooks from 'eslint-plugin-react-hooks'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['dist', 'node_modules', 'src/api/schema.d.ts', 'playwright-report', 'test-results'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.{ts,tsx}'],
    languageOptions: { ecmaVersion: 2023, globals: globals.browser },
    plugins: { 'react-hooks': reactHooks },
    rules: {
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
    },
  },
  {
    // Browser code only; end-to-end tests run in Node.
    files: ['src/**/*.{ts,tsx}'],
    rules: {
      // Self-hosters may test over plain HTTP (e.g. a LAN or tailnet address),
      // where secure-context-only APIs are undefined and crash the page.
      'no-restricted-properties': [
        'error',
        ...[
          ['crypto', 'randomUUID'],
          ['crypto', 'subtle'],
          ['navigator', 'clipboard'],
          ['navigator', 'serviceWorker'],
          ['navigator', 'share'],
          ['navigator', 'locks'],
          ['navigator', 'credentials'],
          ['navigator', 'mediaDevices'],
        ].map(([object, property]) => ({
          object,
          property,
          message: 'Only available over HTTPS; feature-detect it and provide a fallback that works over plain HTTP.',
        })),
      ],
    },
  },
  {
    // All interface text must go through i18next. Generated shadcn/ui
    // primitives contain no user-facing text.
    files: ['src/**/*.tsx'],
    ignores: ['src/components/ui/**'],
    plugins: { i18next },
    rules: {
      'i18next/no-literal-string': ['error', { mode: 'jsx-only', callees: { exclude: ['t', 'i18n(ext)?', 'form.register', 'form.getValues'] }, 'jsx-attributes': { include: ['alt', 'aria-label', 'title', 'placeholder', 'label'] } }],
    },
  },
)
