import * as Haptics from 'expo-haptics';
import { Alert } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage, i18n } from '@/core/i18n';

/** Message of a failed call: this feature's own wording first, then the shared error catalog. */
export function failMessage(e: unknown) {
  if (!isApiError(e)) return i18n.t('common:error_generic');
  if (e.code === 'NETWORK_ERROR') return i18n.t('common:error_network');
  const own = `sales:errors.${e.code}`;
  return i18n.exists(own) ? i18n.t(own) : errorMessage(e.code, e.message);
}

/**
 * Alert for a failed action. A step-up (re-authentication) is a web-only
 * flow: the user is told to finish the action on the web panel.
 */
export function alertError(e: unknown) {
  void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
  if (isApiError(e, 'STEP_UP_REQUIRED')) {
    Alert.alert(i18n.t('sales:step_up_title'), i18n.t('sales:step_up_body'));
    return;
  }
  Alert.alert(failMessage(e));
}

export function success() {
  void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
}
