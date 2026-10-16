// https://docs.expo.dev/guides/using-eslint/
const { defineConfig } = require('eslint/config');
const expoConfig = require('eslint-config-expo/flat');

module.exports = defineConfig([
  expoConfig,
  {
    ignores: ['dist/*', 'src/core/api/schema.d.ts', 'src/uniwind-types.d.ts'],
  },
  {
    rules: {
      'no-restricted-imports': [
        'error',
        {
          paths: [
            {
              name: 'lucide-react-native',
              message: "Import icons from '@/ui/icons' (one file per icon; the root ships every icon).",
              allowTypeImports: true,
            },
          ],
        },
      ],
    },
  },
  {
    files: ['src/ui/icons.ts'],
    rules: { 'no-restricted-imports': 'off' },
  },
]);
