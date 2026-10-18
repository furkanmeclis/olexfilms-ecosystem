import { create } from 'zustand';

import { readJSON, writeJSON } from '@/core/storage';

import { addLine, type CartLine } from './order-lines';

/** Kept on the device so a cart survives leaving the screen or closing the app. */
const KEY = 'orders.cart.v1';

type Cart = {
  lines: CartLine[];
  note: string;
  add: (product: { uuid: string; sku: string; name: string; unit_type: string }) => void;
  setAmount: (productUuid: string, amount: string) => void;
  remove: (productUuid: string) => void;
  setNote: (note: string) => void;
  clear: () => void;
};

const saved = readJSON<Pick<Cart, 'lines' | 'note'>>(KEY);

export const useCart = create<Cart>((set, get) => {
  const persist = () => writeJSON(KEY, { lines: get().lines, note: get().note });
  return {
    lines: saved?.lines ?? [],
    note: saved?.note ?? '',
    add: (product) => {
      set({ lines: addLine(get().lines, product) });
      persist();
    },
    setAmount: (productUuid, amount) => {
      set({ lines: get().lines.map((l) => (l.product_uuid === productUuid ? { ...l, amount } : l)) });
      persist();
    },
    remove: (productUuid) => {
      set({ lines: get().lines.filter((l) => l.product_uuid !== productUuid) });
      persist();
    },
    setNote: (note) => {
      set({ note });
      persist();
    },
    clear: () => {
      set({ lines: [], note: '' });
      persist();
    },
  };
});
