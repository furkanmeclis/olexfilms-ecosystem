import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { GROUPS, ITEMS, itemVisible, menuGroups, type MenuGroup, type MenuViewer } from './groups.ts';

const viewer = (over: Partial<MenuViewer>): MenuViewer => ({
  space: 'bayi',
  workplace: false,
  orgType: 'dealer',
  isSuperAdmin: false,
  permissions: [],
  moduleOn: () => true,
  warehouse: false,
  multiOrg: false,
  ...over,
});

const keysOf = (gs: MenuGroup[]) => gs.flatMap((g) => g.rows.map((r) => (r.kind === 'item' ? r.item.key : `account.${r.row.key}`)));
const layout = (gs: MenuGroup[]) =>
  Object.fromEntries(gs.map((g) => [g.key, g.rows.map((r) => (r.kind === 'item' ? r.item.key : r.row.key))]));

// Menu-relevant grants of the seeded system roles (backend/internal/platform/rbac/catalog.go).
const AI = ['ai.use', 'ai.actions.confirm'];
const ROLES: Record<string, Partial<MenuViewer>> = {
  dealer_owner: {
    permissions: [
      'organizations.read', 'customers.read', 'appointments.read', 'stock.read', 'product_sales.write', 'dealer_pricing.write',
      'purchases.write', 'orders.read', 'catalog.read', 'transfers.request', 'warranty_claims.read', 'leads.read', 'fleets.read',
      'service_subscriptions.read', 'staff.manage', 'accounting.read', 'certificates.read', 'pricing.recommended.read',
      'performance.read', 'measurements.read', 'announcements.read', 'campaigns.read', ...AI,
    ],
  },
  dealer_staff: {
    permissions: ['customers.read', 'appointments.read', 'orders.read', 'catalog.read', 'warranty_claims.read', 'leads.read', 'fleets.read', 'measurements.read', 'announcements.read', ...AI],
  },
  dealer_accounting: {
    permissions: [
      'catalog.read', 'accounting.read', 'pricing.recommended.read', 'orders.read', 'service_subscriptions.read', 'announcements.read',
      'dealer_pricing.write', 'product_sales.write', 'purchases.write', 'staff.manage', ...AI,
    ],
  },
  center_staff: {
    orgType: 'center',
    workplace: true,
    permissions: [
      'catalog.read', 'organizations.read', 'customers.read', 'pricing.recommended.read', 'orders.read', 'tasks.read', 'service_subscriptions.read',
      'measurements.read', 'announcements.read', 'leads.read', 'appointments.read', 'warranty_claims.read', 'certificates.read', 'fleets.read',
      'performance.read', 'pricing.discipline.read', ...AI,
    ],
  },
  center_warehouse: {
    orgType: 'center',
    workplace: true,
    permissions: ['catalog.read', 'organizations.read', 'stock.read', 'orders.read', 'tasks.read', 'announcements.read', ...AI],
  },
  center_accounting: {
    orgType: 'center',
    workplace: true,
    permissions: [
      'catalog.read', 'organizations.read', 'accounting.read', 'pricing.recommended.read', 'orders.read', 'tasks.read', 'service_subscriptions.read',
      'announcements.read', 'einvoice.read', ...AI,
    ],
  },
  center_social: {
    orgType: 'center',
    workplace: true,
    permissions: ['catalog.read', 'organizations.read', 'customers.read', 'campaigns.read', 'leads.read', 'tasks.read', 'announcements.read', 'appointments.read', ...AI],
  },
  distributor_owner: {
    space: 'admin',
    orgType: 'distributor',
    permissions: [
      'catalog.read', 'organizations.read', 'customers.read', 'pricing.recommended.read', 'accounting.read', 'stock.read', 'orders.read',
      'transfers.request', 'service_subscriptions.read', 'measurements.read', 'announcements.read', 'leads.read', 'appointments.read',
      'warranty_claims.read', 'certificates.read', 'campaigns.read', 'fleets.read', 'performance.read', 'pricing.discipline.read', ...AI,
    ],
  },
  distributor_staff: {
    space: 'admin',
    orgType: 'distributor',
    permissions: ['catalog.read', 'organizations.read', 'customers.read', 'measurements.read', 'announcements.read', 'leads.read', 'appointments.read', 'warranty_claims.read', 'performance.read', ...AI],
  },
  distributor_accounting: {
    space: 'admin',
    orgType: 'distributor',
    permissions: ['catalog.read', 'accounting.read', 'pricing.recommended.read', 'orders.read', 'service_subscriptions.read', 'announcements.read', ...AI],
  },
  warehouse_space: { space: 'depo', orgType: 'center', permissions: ['stock.read', 'orders.read', 'announcements.read', ...AI] },
};

describe('menu group definition', () => {
  it('puts every item in exactly one real group', () => {
    const seen = new Set<string>();
    for (const i of ITEMS) {
      assert.ok(!seen.has(i.key), `duplicate item ${i.key}`);
      seen.add(i.key);
      assert.ok(GROUPS.includes(i.group), `${i.key} has no known group`);
      assert.notEqual(i.group, 'other');
      for (const f of i.fallback ?? []) assert.notEqual(f, i.group, `${i.key} falls back to its own group`);
    }
  });

  it('defines no single-item group', () => {
    for (const g of GROUPS) {
      if (g === 'other' || g === 'account') continue;
      assert.ok(ITEMS.filter((i) => i.group === g).length >= 2, `group ${g}`);
    }
  });

  it('shows each item once when everything is visible', () => {
    // A super admin in the center workplace sees every row (bayi + admin + center-only).
    const gs = menuGroups(viewer({ isSuperAdmin: true, workplace: true, orgType: 'center', warehouse: true, multiOrg: true }));
    const keys = keysOf(gs);
    assert.equal(new Set(keys).size, keys.length);
    assert.deepEqual(
      keys.filter((k) => !k.startsWith('account.')).sort(),
      ITEMS.map((i) => i.key).sort(),
    );
    assert.deepEqual(layout(gs), {
      operations: ['approvals', 'appointments', 'occupancy', 'tasks', 'warranty_claims', 'measurements', 'certificates'],
      customers: ['customers', 'leads', 'fleets', 'service_subscriptions'],
      stock: ['orders', 'stock', 'quick_sale', 'transfers', 'purchases', 'catalog'],
      finance: ['accounting', 'disputes', 'sale_prices', 'pricing', 'einvoices'],
      business: ['network', 'staff', 'performance', 'reports', 'campaigns', 'announcements'],
      assistant: ['assistant', 'ai_actions'],
      account: ['warehouse', 'search', 'spaces', 'settings'],
    });
  });
});

describe('menuGroups per role', () => {
  for (const [role, over] of Object.entries(ROLES)) {
    it(`${role}: no empty or lone-row group, account last, group order kept`, () => {
      const v = viewer(over);
      const gs = menuGroups(v);
      for (const g of gs) assert.ok(g.rows.length > 0, `${role} empty ${g.key}`);
      for (const g of gs) if (g.key !== 'account') assert.ok(g.rows.length >= 2, `${role} lone row in ${g.key}`);
      assert.equal(gs.at(-1)?.key, 'account');
      const order = gs.map((g) => GROUPS.indexOf(g.key));
      assert.deepEqual(order, [...order].sort((a, b) => a - b));
      // Exactly the rows the flat menu showed, each once.
      const items = keysOf(gs).filter((k) => !k.startsWith('account.'));
      assert.deepEqual([...items].sort(), ITEMS.filter((i) => itemVisible(i, v)).map((i) => i.key).sort());
      assert.equal(new Set(items).size, items.length);
    });
  }

  it('dealer owner', () => {
    assert.deepEqual(layout(menuGroups(viewer(ROLES.dealer_owner!))), {
      operations: ['appointments', 'warranty_claims'],
      customers: ['customers', 'leads', 'fleets', 'service_subscriptions'],
      stock: ['orders', 'stock', 'quick_sale', 'transfers', 'purchases', 'catalog'],
      finance: ['accounting', 'disputes', 'sale_prices'],
      business: ['staff', 'reports', 'campaigns', 'announcements'],
      assistant: ['assistant', 'ai_actions'],
      account: ['search', 'settings'],
    });
  });

  it('dealer staff', () => {
    assert.deepEqual(layout(menuGroups(viewer(ROLES.dealer_staff!))), {
      operations: ['appointments', 'warranty_claims'],
      customers: ['customers', 'leads', 'fleets'],
      stock: ['orders', 'catalog'],
      business: ['reports', 'announcements'],
      assistant: ['assistant', 'ai_actions'],
      account: ['search', 'settings'],
    });
  });

  it('accounting roles: subscriptions (and lone orders) join finance', () => {
    const dealer = layout(menuGroups(viewer(ROLES.dealer_accounting!)));
    assert.deepEqual(dealer.finance, ['service_subscriptions', 'accounting', 'disputes', 'sale_prices']);
    assert.equal(dealer.customers, undefined);
    const dist = layout(menuGroups(viewer(ROLES.distributor_accounting!)));
    assert.deepEqual(dist.finance, ['service_subscriptions', 'orders', 'accounting', 'disputes', 'pricing']);
  });

  it('lone orders join operations before finance', () => {
    const gs = layout(menuGroups(viewer({ permissions: ['orders.read', 'appointments.read', 'warranty_claims.read'] })));
    assert.deepEqual(gs.operations, ['appointments', 'warranty_claims', 'orders']);
    assert.equal(gs.stock, undefined);
  });

  it('warehouse space: one untitled list over account', () => {
    const gs = menuGroups(viewer(ROLES.warehouse_space!));
    assert.deepEqual(layout(gs), { other: ['orders', 'announcements'], account: ['search', 'settings'] });
    assert.equal(gs[0]!.titled, false);
    assert.equal(gs[1]!.titled, true);
  });
});

describe('hidden rows stay hidden', () => {
  it('drops rows without their permission and the groups they emptied', () => {
    // Reports need no permission; everything else here is missing.
    const gs = menuGroups(viewer({ permissions: ['customers.read', 'leads.read', 'announcements.read'] }));
    assert.deepEqual(layout(gs), {
      customers: ['customers', 'leads'],
      business: ['reports', 'announcements'],
      account: ['search', 'settings'],
    });
    assert.ok(gs.every((g) => g.rows.length > 0));
  });

  it('a single leftover row keeps its own header; several share "other"', () => {
    assert.deepEqual(layout(menuGroups(viewer({ permissions: ['customers.read', 'leads.read'] }))), {
      customers: ['customers', 'leads'],
      business: ['reports'],
      account: ['search', 'settings'],
    });
    assert.deepEqual(layout(menuGroups(viewer({ permissions: ['customers.read', 'leads.read', 'stock.read'] }))), {
      customers: ['customers', 'leads'],
      other: ['stock', 'reports'],
      account: ['search', 'settings'],
    });
  });

  it('drops a switched-off module (e.g. appointments)', () => {
    const v = viewer({ ...ROLES.dealer_owner!, moduleOn: (k) => k !== 'appointments' });
    const gs = layout(menuGroups(v));
    assert.ok(!Object.values(gs).flat().includes('appointments'));
    // The warranty row would be alone in operations, so it joins customers.
    assert.equal(gs.operations, undefined);
    assert.ok(gs.customers!.includes('warranty_claims'));
  });

  it('drops everything for a viewer with no rows but keeps account', () => {
    const gs = menuGroups(viewer({ moduleOn: () => false }));
    assert.deepEqual(layout(gs), { account: ['search', 'settings'] });
  });

  it('keeps space, org type and workplace rules', () => {
    const dealer = keysOf(menuGroups(viewer({ isSuperAdmin: true })));
    for (const k of ['approvals', 'network', 'einvoices', 'tasks', 'occupancy', 'certificates']) assert.ok(!dealer.includes(k), k);
    const distributor = keysOf(menuGroups(viewer({ isSuperAdmin: true, space: 'admin', orgType: 'distributor' })));
    for (const k of ['einvoices', 'approvals', 'network', 'appointments', 'quick_sale']) assert.ok(!distributor.includes(k), k);
    assert.ok(distributor.includes('tasks'));
  });

  it('shows warehouse and spaces rows only when allowed', () => {
    assert.deepEqual(layout(menuGroups(viewer({}))).account, ['search', 'settings']);
    assert.deepEqual(layout(menuGroups(viewer({ warehouse: true, multiOrg: true }))).account, ['warehouse', 'search', 'spaces', 'settings']);
  });
});
