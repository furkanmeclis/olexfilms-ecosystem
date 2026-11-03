import type { SpaceKind } from '@/core/spaces';
import type { IconName } from '@/ui/icons';

/**
 * Menu groups: one definition for every space. Pure (no React, no session) so
 * node:test can cover it; the screen only feeds in who is looking.
 */

export type OrgType = 'center' | 'distributor' | 'dealer';

/** Group keys in screen order: the busiest work on top, account and settings last. */
export const GROUPS = ['operations', 'customers', 'stock', 'finance', 'business', 'assistant', 'other', 'account'] as const;
export type GroupKey = (typeof GROUPS)[number];

export type MenuItem = {
  /** Also the route name under `/<space>/menu/` and the module filter key. */
  key: string;
  icon: IconName;
  group: Exclude<GroupKey, 'other'>;
  /**
   * Groups the row moves to, in order, when it would be the only row of its own
   * group (e.g. a dealer staff member sees orders but no other stock rows).
   */
  fallback?: Exclude<GroupKey, 'other' | 'account'>[];
  permission?: string | string[];
  spaces: SpaceKind[];
  built?: SpaceKind[];
  /** Organization types the backend serves the screen to (e.g. e-invoices: the brand center only). */
  orgTypes?: OrgType[];
};

/** Account rows: not module screens; each opens its own place (see the screen). */
export type AccountRow = { key: 'warehouse' | 'search' | 'spaces' | 'settings'; icon: IconName };

/**
 * Menu rows appear only with the permission (and so the module) behind them:
 * a closed module is not drawn at all. Rows open their screens as they are built.
 * Order inside a group is the order here.
 */
/**
 * Who has a section on the approvals screen: orders, module requests, stock
 * reclassifications, warranty claims to review. Without any of them the
 * screen would only say "nothing for you", so its shortcuts stay hidden.
 */
export const APPROVAL_PERMISSIONS = ['orders.approve', 'modules.manage', 'stock.reclassify', 'warranty_claims.review'];

export const ITEMS: MenuItem[] = [
  // Operations: the day's queue.
  // TEC-1016: the center works in the dealer workplace; its admin tabs (approvals, network) become menu rows.
  {
    key: 'approvals',
    icon: 'warrantyClaim',
    group: 'operations',
    permission: APPROVAL_PERMISSIONS,
    spaces: [],
    built: [],
    orgTypes: ['center'],
  },
  { key: 'appointments', icon: 'appointment', group: 'operations', permission: 'appointments.read', spaces: ['bayi'], built: ['bayi'] },
  { key: 'occupancy', icon: 'calendar', group: 'operations', permission: 'appointments.read', spaces: ['admin'], built: ['admin'] },
  // Alone in operations (e.g. center accounting, no approvals row): next to the business rows.
  { key: 'tasks', icon: 'task', group: 'operations', fallback: ['business'], permission: 'tasks.read', spaces: ['admin'], built: ['admin'] },
  // TEC-866: no warranty list row for dealers (bayi); warranties open from the service detail.
  { key: 'warranty_claims', icon: 'warrantyClaim', group: 'operations', fallback: ['customers'], permission: 'warranty_claims.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'measurements', icon: 'measure', group: 'operations', permission: 'measurements.read', spaces: ['admin'], built: ['admin'] },
  { key: 'certificates', icon: 'shield', group: 'operations', permission: 'certificates.read', spaces: ['admin'], built: ['admin'] },

  // Customers.
  { key: 'customers', icon: 'customers', group: 'customers', permission: 'customers.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'leads', icon: 'lead', group: 'customers', permission: 'leads.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  // TEC-959: dealer fleet management, service subscriptions and staff cards (web: fleets, service-subscriptions, staff).
  { key: 'fleets', icon: 'truck', group: 'customers', permission: 'fleets.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  {
    key: 'service_subscriptions',
    icon: 'subscription',
    group: 'customers',
    // Accounting roles read subscriptions without any customer row.
    fallback: ['finance'],
    permission: 'service_subscriptions.read',
    spaces: ['bayi', 'admin'],
    built: ['bayi', 'admin'],
  },

  // Stock and orders.
  {
    key: 'orders',
    icon: 'order',
    group: 'stock',
    // Without other stock rows: next to appointments for service staff, next to the cari accounts for accountants.
    fallback: ['operations', 'finance'],
    permission: 'orders.read',
    spaces: ['bayi', 'depo', 'admin'],
    built: ['bayi', 'depo', 'admin'],
  },
  // TEC-958: the organization's own stock (the dealer's stock tab gives way to Measure).
  { key: 'stock', icon: 'stock', group: 'stock', permission: 'stock.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  // TEC-958: dealer sales (web Dealer sales; accounting + dealer_accounting modules behind each permission).
  { key: 'quick_sale', icon: 'cart', group: 'stock', permission: 'product_sales.write', spaces: ['bayi'], built: ['bayi'] },
  { key: 'transfers', icon: 'transfer', group: 'stock', permission: 'transfers.request', spaces: ['bayi'], built: ['bayi'] },
  { key: 'purchases', icon: 'truck', group: 'stock', permission: 'purchases.write', spaces: ['bayi'], built: ['bayi'] },
  { key: 'catalog', icon: 'package', group: 'stock', fallback: ['business'], permission: 'catalog.read', spaces: ['bayi'], built: ['bayi'] },

  // Finance.
  // TEC-957: cari accounts and disputes (both sides read with accounting.read); e-invoices in the center.
  { key: 'accounting', icon: 'accounting', group: 'finance', permission: 'accounting.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'disputes', icon: 'alert', group: 'finance', permission: 'accounting.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'sale_prices', icon: 'accounting', group: 'finance', permission: 'dealer_pricing.write', spaces: ['bayi'], built: ['bayi'] },
  {
    key: 'pricing',
    icon: 'accounting',
    group: 'finance',
    // Center staff read price discipline without any accounting row.
    fallback: ['business'],
    permission: ['pricing.recommended.read', 'pricing.discipline.read'],
    spaces: ['admin'],
    built: ['admin'],
  },
  { key: 'einvoices', icon: 'report', group: 'finance', permission: 'einvoice.read', spaces: ['admin'], built: ['admin'], orgTypes: ['center'] },

  // Business: the organization, its people and its numbers.
  { key: 'network', icon: 'network', group: 'business', permission: 'organizations.read', spaces: [], built: [], orgTypes: ['center'] },
  { key: 'staff', icon: 'staff', group: 'business', permission: 'staff.manage', spaces: ['bayi'], built: ['bayi'] },
  { key: 'performance', icon: 'chart', group: 'business', permission: 'performance.read', spaces: ['admin'], built: ['admin'] },
  { key: 'reports', icon: 'report', group: 'business', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  // TEC-960: campaigns (approval queue inside), the panel AI assistant and the AI action approval queue.
  { key: 'campaigns', icon: 'campaign', group: 'business', permission: 'campaigns.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'announcements', icon: 'announcement', group: 'business', permission: 'announcements.read', spaces: ['bayi', 'depo', 'admin'], built: ['bayi', 'depo', 'admin'] },

  // AI assistant.
  { key: 'assistant', icon: 'assistant', group: 'assistant', fallback: ['business'], permission: 'ai.use', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'ai_actions', icon: 'check', group: 'assistant', fallback: ['business'], permission: 'ai.actions.confirm', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
];

export const ACCOUNT_ROWS: AccountRow[] = [
  { key: 'warehouse', icon: 'warehouse' },
  { key: 'search', icon: 'search' },
  { key: 'spaces', icon: 'spaces' },
  { key: 'settings', icon: 'settings' },
];

/** Who is looking: everything the visibility rules read, already resolved by the screen. */
export type MenuViewer = {
  space: SpaceKind;
  /** The center works in the dealer workplace with the admin rows in its menu (TEC-1016). */
  workplace: boolean;
  orgType?: string;
  isSuperAdmin: boolean;
  permissions: readonly string[];
  /** False for a switched-off module (keyed like the route). */
  moduleOn: (key: string) => boolean;
  /** Warehouse screens open over this space (switch row). */
  warehouse: boolean;
  /** More than one organization to switch between. */
  multiOrg: boolean;
};

export function can(viewer: Pick<MenuViewer, 'isSuperAdmin' | 'permissions'>, p?: string | string[]): boolean {
  return !p || viewer.isSuperAdmin || [p].flat().some((k) => viewer.permissions.includes(k));
}

/** Same condition the flat menu used before grouping: space, permission, module and org type. */
export function itemVisible(i: MenuItem, v: MenuViewer): boolean {
  const shows = i.spaces.includes(v.space) || (v.workplace && (i.spaces.includes('admin') || i.spaces.length === 0));
  return (
    shows &&
    can(v, i.permission) &&
    v.moduleOn(i.key) &&
    (!i.orgTypes || (!!v.orgType && (i.orgTypes as string[]).includes(v.orgType)))
  );
}

/** Whether the row opens its screen in this space (else the "soon" placeholder). */
export function itemBuilt(i: MenuItem, v: Pick<MenuViewer, 'space' | 'workplace'>): boolean {
  return !!i.built?.includes(v.space) || (v.workplace && (!!i.built?.includes('admin') || i.spaces.length === 0));
}

export function accountRowVisible(r: AccountRow, v: MenuViewer): boolean {
  if (r.key === 'warehouse') return v.warehouse;
  if (r.key === 'spaces') return v.multiOrg;
  return true;
}

export type MenuRow = { kind: 'item'; item: MenuItem } | { kind: 'account'; row: AccountRow };
export type MenuGroup = {
  key: GroupKey;
  /** False for an "other" group that is the page's only module group. */
  titled: boolean;
  rows: MenuRow[];
};

/**
 * Visible rows grouped for the viewer. Empty groups are dropped. A row that
 * would sit alone in its group moves to its first fallback group that has other
 * rows; when several rows are still alone they end up together in "other".
 */
export function menuGroups(v: MenuViewer): MenuGroup[] {
  const byGroup = new Map<GroupKey, MenuItem[]>(GROUPS.map((g) => [g, []]));
  for (const i of ITEMS) if (itemVisible(i, v)) byGroup.get(i.group)!.push(i);

  // Fold lone rows into a fallback group, in group order, so the result is stable.
  for (const g of GROUPS) {
    const rows = byGroup.get(g)!;
    if (rows.length !== 1) continue;
    const lone = rows[0]!;
    const target = lone.fallback?.find((f) => byGroup.get(f)!.length > 0);
    if (!target) continue;
    rows.length = 0;
    insertInOrder(byGroup.get(target)!, lone);
  }
  // Two or more lone rows share "other"; a single one keeps its own, more telling header.
  const lone = GROUPS.filter((g) => g !== 'other' && g !== 'account' && byGroup.get(g)!.length === 1);
  if (lone.length > 1) {
    for (const g of lone) {
      const rows = byGroup.get(g)!;
      insertInOrder(byGroup.get('other')!, rows[0]!);
      rows.length = 0;
    }
  }

  const modules = GROUPS.filter((g) => g !== 'account' && byGroup.get(g)!.length > 0);
  const out: MenuGroup[] = modules.map((g) => ({
    key: g,
    // "Other" only names leftovers next to real groups; alone (e.g. the warehouse) it needs no header.
    titled: g !== 'other' || modules.length > 1,
    rows: byGroup.get(g)!.map((item) => ({ kind: 'item', item })),
  }));
  const account = ACCOUNT_ROWS.filter((r) => accountRowVisible(r, v));
  if (account.length) out.push({ key: 'account', titled: true, rows: account.map((row) => ({ kind: 'account', row })) });
  return out;
}

/** Keep a group's rows in definition order after a move. */
function insertInOrder(rows: MenuItem[], item: MenuItem) {
  const rank = (i: MenuItem) => ITEMS.indexOf(i);
  const at = rows.findIndex((r) => rank(r) > rank(item));
  if (at === -1) rows.push(item);
  else rows.splice(at, 0, item);
}
