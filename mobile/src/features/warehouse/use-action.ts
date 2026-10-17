import { useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { useState } from 'react';

import { alertError } from './format';

/**
 * Runs one document action at a time: tracks which one is busy, puts the
 * returned document into the cache, refreshes the lists and reports errors.
 */
export function useDocAction<T>(key: readonly unknown[]) {
  const qc = useQueryClient();
  const [busy, setBusy] = useState<string | null>(null);

  const run = async (name: string, fn: () => Promise<T | void>, opts: { done?: () => void } = {}) => {
    if (busy) return;
    setBusy(name);
    try {
      const out = await fn();
      if (out !== undefined) qc.setQueryData(key, out);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      // Lists and counters refresh in the background; the document itself is already current.
      void qc.invalidateQueries({ queryKey: ['warehouse'], predicate: (q) => q.queryKey !== key && q.queryKey[1] !== key[1] });
      void qc.invalidateQueries({ queryKey: ['stock'] });
      opts.done?.();
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(null);
    }
  };

  return { busy, run };
}
