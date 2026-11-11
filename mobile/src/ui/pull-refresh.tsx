import { useEffect, useRef, useState } from 'react';
import { RefreshControl, type RefreshControlProps } from 'react-native';

type Props = Omit<RefreshControlProps, 'refreshing' | 'onRefresh'> & {
  /** The data is being fetched (e.g. `query.isRefetching`); keeps a pull's spinner until it ends. */
  busy?: boolean;
  /** Called on pull; a returned promise keeps the spinner until it settles. */
  onRefresh: () => unknown;
};

/** Spinner fallback when neither a promise nor `busy` tells when the refresh ends. */
const FALLBACK_MS = 800;

/**
 * Pull-to-refresh that only spins for the user's own pull. A `refreshing` prop
 * that follows background refetches (screen focus, cached data revalidated on
 * mount) makes iOS push the content down under a transparent header and not
 * give the inset back, leaving an empty band above the list. Drop-in for
 * RefreshControl (Android clones it with the scroll view as its child).
 */
export function PullRefresh({ busy = false, onRefresh, ...rest }: Props) {
  const [pulling, setPulling] = useState(false);
  const sawBusy = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!pulling) return;
    if (busy) sawBusy.current = true;
    else if (sawBusy.current) setPulling(false);
  }, [busy, pulling]);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  const handle = () => {
    sawBusy.current = false;
    setPulling(true);
    const out = onRefresh();
    if (out && typeof (out as Promise<unknown>).then === 'function') {
      void Promise.resolve(out)
        .catch(() => undefined)
        .finally(() => setPulling(false));
      return;
    }
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      if (!sawBusy.current) setPulling(false);
    }, FALLBACK_MS);
  };

  return <RefreshControl {...rest} refreshing={pulling} onRefresh={handle} />;
}
