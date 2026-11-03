import { Alert } from 'react-native';

import { i18n } from '@/core/i18n';

/** Asks before an irreversible step (one shared pattern for every screen). */
export function confirmAction(opts: { title: string; body?: string; action: string; destructive?: boolean; onConfirm: () => void }) {
  Alert.alert(opts.title, opts.body, [
    { text: i18n.t('common:cancel'), style: 'cancel' },
    { text: opts.action, style: opts.destructive ? 'destructive' : 'default', onPress: opts.onConfirm },
  ]);
}
