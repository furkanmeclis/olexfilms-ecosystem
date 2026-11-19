import { create } from 'zustand';

import type { MobileMe } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';
import { readJSON, storage, writeJSON } from '@/core/storage';

import { addLine, type CartLine, type CartProduct, convertToBoxes, lineKey, lineMeasure, type SaleMeasure } from './order-lines';
import type { SupplementParent } from './supplement';

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

/** `parent`: the cart is a supplementary order of that shipped order (TEC-1222). */
type Saved = { lines: CartLine[]; note: string; parent?: SupplementParent | null };
const EMPTY: Saved = { lines: [], note: '', parent: null };

function load(key: string | null): Saved {
  return { ...EMPTY, ...((key && readJSON<Saved>(key)) || {}) };
}

/**
 * Line actions take the line key (product + measure, TEC-1240): the same
 * product may have a box and a meter line (K10); a line's measure never changes.
 */
type Cart = Saved & {
  key: string | null;
  /** Adds the product in that measure (default: its natural one); a measure it is not sold by is ignored. */
  add: (product: CartProduct, measure?: SaleMeasure) => void;
  setAmount: (lineKey: string, amount: string) => void;
  setPrice: (lineKey: string, price: string) => void;
  /** TEC-1059: the center marks a line "sent plotter cut". */
  setPlotter: (lineKey: string, plotterCut: boolean) => void;
  /** TEC-1221: usage credits of a plotter cut line's barcodes (raw input). */
  setPlotterCredits: (lineKey: string, credits: string) => void;
  /** TEC-1240 (K3/K12): credits of each cut piece of a meter line (raw input). */
  setLineCredits: (lineKey: string, credits: string) => void;
  setDiscount: (lineKey: string, discount: string) => void;
  setGift: (lineKey: string, gift: boolean) => void;
  /** TEC-1240: applies the whole box suggestion of a meter line (one tap). */
  toBoxes: (lineKey: string) => void;
  remove: (lineKey: string) => void;
  setNote: (note: string) => void;
  /** TEC-1222: start (empty cart) or leave a supplementary order. */
  startSupplement: (parent: SupplementParent) => void;
  leaveSupplement: () => void;
  clear: () => void;
};

const initialKey = cartKey(useSession.getState().me);

export const useCart = create<Cart>((set, get) => {
  const update = (next: Partial<Saved>) => {
    set(next);
    const { key, lines, note, parent } = get();
    if (key) writeJSON(key, { lines, note, parent });
  };
  const patch = (k: string, p: Partial<CartLine>) =>
    update({ lines: get().lines.map((l) => (lineKey(l) === k ? { ...l, ...p } : l)) });
  return {
    key: initialKey,
    ...load(initialKey),
    add: (product, measure) => update({ lines: addLine(get().lines, product, measure) }),
    setAmount: (k, amount) => patch(k, { amount }),
    setPrice: (k, unit_price) => patch(k, { unit_price }),
    setPlotter: (k, plotter_cut) => patch(k, { plotter_cut }),
    setPlotterCredits: (k, plotter_credits) => patch(k, { plotter_credits }),
    setLineCredits: (k, line_credits) => patch(k, { line_credits }),
    setDiscount: (k, discount_pct) => patch(k, { discount_pct }),
    setGift: (k, is_gift) => patch(k, { is_gift }),
    toBoxes: (k) => update({ lines: convertToBoxes(get().lines, k) }),
    remove: (k) => update({ lines: get().lines.filter((l) => lineKey(l) !== k) }),
    setNote: (note) => update({ note }),
    startSupplement: (parent) => update({ ...EMPTY, parent }),
    leaveSupplement: () => update({ parent: null }),
    clear: () => update(EMPTY),
  };
});

/** Whether the cart holds the product in that measure (catalog marks). */
export const inCart = (lines: readonly CartLine[], productUuid: string, measure: SaleMeasure) =>
  lines.some((l) => l.product_uuid === productUuid && lineMeasure(l) === measure);

// Follow the session: switching organization loads that organization's cart,
// signing out drops the cart from memory and from the device.
useSession.subscribe((s) => {
  const key = cartKey(s.me);
  const prev = useCart.getState().key;
  if (key === prev) return;
  if (s.status === 'signedOut' && prev) storage.remove(prev);
  useCart.setState({ key, ...load(key) });
});
