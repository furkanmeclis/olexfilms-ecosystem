import { I18nManager, Platform } from 'react-native';

/**
 * Native large titles, except on iOS in right-to-left: there react-native-screens
 * forces the bar's semantic direction on top of UIKit's own RTL and the large
 * title is drawn mirrored. Compact titles render correctly.
 */
export const LARGE_TITLES = !(Platform.OS === 'ios' && I18nManager.isRTL);
