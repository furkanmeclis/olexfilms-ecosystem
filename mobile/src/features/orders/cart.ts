import { create } from 'zustand';

import type { MobileMe } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';
import { readJSON, storage, writeJSON } from '@/core/storage';

import { addLine, type CartLine } from './order-lines';

/** First cart key: one cart for the whole device (shared between users). Removed on load. */
const LEGACY_KEY = 'orders.cart.v1';
storage.remove(LEGACY_KEY);

/**
 * A cart belongs to one user in one organization: another user of the same
 * device, or the same user in another organization, never sees it.
 */
function cartKey(me: MobileMe | null): string | null {
  return me?.user.uuid && me.active_organization_uuid ? `orders.cart.v2.${me.user.uuid}.${me.active_organization_uuid}` : null;
}

type Saved = { lines: CartLine[]; note: string };
const EMPTY: Saved = { lines: [], note: '' };

function load(key: string | null): Saved {
  return (key && readJSON<Saved>(key)) || EMPTY;
}

type Cart = Saved & {
  key: string | null;
  add: (product: { uuid: string; sku: string; name: string; unit_type: string }) => void;
  setAmount: (productUuid: string, amount: string) => void;
  remove: (productUuid: string) => void;
  setNote: (note: string) => void;
  clear: () => void;
};

const initialKey = cartKey(useSession.getState().me);

export const useCart = create<Cart>((set, get) => {
  const update = (next: Partial<Saved>) => {
    set(next);
    const { key, lines, note } = get();
    if (key) writeJSON(key, { lines, note });
  };
  return {
    key: initialKey,
    ...load(initialKey),
    add: (product) => update({ lines: addLine(get().lines, product) }),
    setAmount: (productUuid, amount) =>
      update({ lines: get().lines.map((l) => (l.product_uuid === productUuid ? { ...l, amount } : l)) }),
    remove: (productUuid) => update({ lines: get().lines.filter((l) => l.product_uuid !== productUuid) }),
    setNote: (note) => update({ note }),
    clear: () => update(EMPTY),
  };
});

// Follow the session: switching organization loads that organization's cart,
// signing out drops the cart from memory and from the device.
useSession.subscribe((s) => {
  const key = cartKey(s.me);
  const prev = useCart.getState().key;
  if (key === prev) return;
  if (s.status === 'signedOut' && prev) storage.remove(prev);
  useCart.setState({ key, ...load(key) });
});
