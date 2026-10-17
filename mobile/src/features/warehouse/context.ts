import { create } from 'zustand';

import { useSession } from '@/core/auth/session';
import { readJSON, writeJSON } from '@/core/storage';

import { useWarehouses, type Warehouse } from './api';

/**
 * The warehouse the user works in, per organization. The backend has no
 * warehouse context yet (`whid` is reserved), so the choice lives on the
 * device and is sent with each request that needs it.
 */
const KEY = 'warehouse.selected';

type Saved = Record<string, string>;

type Store = {
  byOrg: Saved;
  select: (org: string, warehouse: string) => void;
};

export const useWarehouseChoice = create<Store>((set, get) => ({
  byOrg: readJSON<Saved>(KEY) ?? {},
  select: (org, warehouse) => {
    const byOrg = { ...get().byOrg, [org]: warehouse };
    writeJSON(KEY, byOrg);
    set({ byOrg });
  },
}));

/**
 * Current warehouse: the saved one if still active, else the first active
 * warehouse of the organization.
 */
export function useCurrentWarehouse(): {
  warehouse: Warehouse | undefined;
  warehouses: Warehouse[];
  isLoading: boolean;
  isError: boolean;
  refetch: () => void;
  select: (uuid: string) => void;
} {
  const org = useSession((s) => s.me?.active_organization_uuid ?? '');
  const saved = useWarehouseChoice((s) => s.byOrg[org]);
  const select = useWarehouseChoice((s) => s.select);
  const q = useWarehouses();
  const list = q.data ?? [];
  const warehouse = list.find((w) => w.uuid === saved) ?? list[0];
  return {
    warehouse,
    warehouses: list,
    isLoading: q.isLoading,
    isError: q.isError,
    refetch: () => void q.refetch(),
    select: (uuid) => select(org, uuid),
  };
}
