const path = require('node:path');
const { getDefaultConfig } = require('expo/metro-config');
const { withUniwindConfig } = require('uniwind/metro');

const config = getDefaultConfig(__dirname);

// API error messages come from the web catalogs (frontend/src/locales/*/errors.json)
// so a backend error code is translated the same way on web and mobile.
config.watchFolders = [path.resolve(__dirname, '../frontend/src/locales')];

// withUniwindConfig must stay the outermost wrapper.
module.exports = withUniwindConfig(config, {
  cssEntryFile: './src/global.css',
  dtsFile: './src/uniwind-types.d.ts',
});
