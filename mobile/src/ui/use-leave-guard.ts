import { usePreventRemove } from 'expo-router';

import { i18n } from '@/core/i18n';

import { confirmAction } from './confirm';

/**
 * Asks before the screen is left (back button, swipe, header back) while
 * `dirty` holds, so typed or picked values are not lost silently. Returns
 * `release`: call it right before navigating away after a successful save,
 * so that navigation is not asked about.
 */
export function useLeaveGuard(dirty: boolean, opts?: { title?: string; body?: string }) {
  return usePreventRemove(dirty, ({ repeat }) =>
    confirmAction({
      title: opts?.title ?? i18n.t('common:leave_title'),
      body: opts?.body ?? i18n.t('common:leave_body'),
      action: i18n.t('common:leave_discard'),
      destructive: true,
      onConfirm: repeat,
    }),
  );
}
