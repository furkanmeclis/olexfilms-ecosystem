import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { ENTITY_KINDS, entityPath, type HrefContext, KIND_ICONS, menuRoutePath, stackPath } from './href.ts';

const ALL = [
  'services.read', 'customers.read', 'warranties.read', 'orders.read', 'leads.read', 'warranty_claims.read', 'accounting.read',
  'einvoice.read', 'tasks.read', 'appointments.read', 'measurements.read', 'announcements.read', 'service_subscriptions.read',
  'transfers.request', 'organizations.read', 'catalog.read', 'stock.read',
];

const ctx = (segments: string[], over: Partial<HrefContext> = {}): HrefContext => ({
  segments,
  fallbackSpace: 'bayi',
  can: (p) => ALL.includes(p),
  routeOn: () => true,
  orgType: 'dealer',
  ...over,
});

describe('stackPath (old useLinks, TEC-1219)', () => {
  it('keeps services, customers and warranties inside the current stack', () => {
    const services = ['(app)', 'bayi', 'services', '[uuid]'];
    assert.equal(stackPath(services, 'services', 's1'), '/bayi/services/s1');
    assert.equal(stackPath(services, 'customers', 'c1'), '/bayi/services/customers/c1');
    assert.equal(stackPath(services, 'warranties', 'w1'), '/bayi/services/warranties/w1');
    const customers = ['(app)', 'bayi', 'menu', 'customers', '[uuid]'];
    assert.equal(stackPath(customers, 'customers', 'c1'), '/bayi/menu/customers/c1');
    assert.equal(stackPath(customers, 'services', 's1'), '/bayi/menu/customers/services/s1');
    const warranties = ['(app)', 'bayi', 'menu', 'warranties'];
    assert.equal(stackPath(warranties, 'warranties', 'w1'), '/bayi/menu/warranties/w1');
    assert.equal(stackPath(warranties, 'customers', 'c1'), '/bayi/menu/warranties/customers/c1');
  });

  it('opens the services tab from home and other tabs', () => {
    assert.equal(stackPath(['(app)', 'bayi', '(home)'], 'services', 's1'), '/bayi/services/s1');
    assert.equal(stackPath(['(app)', 'bayi', 'menu', 'orders'], 'customers', 'c1'), '/bayi/services/customers/c1');
  });

  it('uses the customers menu stack in the admin space (no services tab there)', () => {
    assert.equal(stackPath(['(app)', 'admin', '(home)'], 'services', 's1'), '/admin/menu/customers/services/s1');
    assert.equal(stackPath(['(app)', 'admin', 'network'], 'customers', 'c1'), '/admin/menu/customers/c1');
  });
});

describe('entityPath', () => {
  it('opens what the search screen opened before, from outside a space', () => {
    const search = ['(app)', 'search'];
    assert.equal(entityPath(ctx(search), 'service', 's1'), '/bayi/services/s1');
    assert.equal(entityPath(ctx(search), 'customer', 'c1'), '/bayi/menu/customers/c1');
    assert.equal(entityPath(ctx(search), 'warranty', 'w1'), '/bayi/menu/warranties/w1');
    assert.equal(entityPath(ctx(search), 'order', 'o1'), '/bayi/menu/orders/o1');
    const admin = ctx(search, { fallbackSpace: 'admin' });
    assert.equal(entityPath(admin, 'service', 's1'), '/admin/menu/customers/services/s1');
    assert.equal(entityPath(admin, 'customer', 'c1'), '/admin/menu/customers/c1');
    assert.equal(entityPath(admin, 'warranty', 'w1'), '/admin/menu/customers/warranties/w1');
    assert.equal(entityPath(admin, 'order', 'o1'), '/admin/menu/orders/o1');
  });

  it('opens menu kinds in the space menu', () => {
    const svc = ['(app)', 'bayi', 'services', '[uuid]'];
    assert.equal(entityPath(ctx(svc), 'lead', 'l1'), '/bayi/menu/leads/l1');
    assert.equal(entityPath(ctx(svc), 'task', 't1'), '/bayi/menu/tasks/t1');
    assert.equal(entityPath(ctx(svc), 'transfer', 'x1'), '/bayi/menu/transfers/x1');
    assert.equal(entityPath(ctx(svc), 'cari', 'k1'), '/bayi/menu/accounting/k1');
    assert.equal(entityPath(ctx(['(app)', 'depo', 'jobs']), 'order', 'o1'), '/depo/menu/orders/o1');
    assert.equal(entityPath(ctx(['(app)', 'bayi', 'orders', '[uuid]']), 'order', 'o2'), '/bayi/orders/o2');
  });

  it('gives null without the permission, with the module off or without a screen in the space', () => {
    const svc = ['(app)', 'bayi', 'services'];
    assert.equal(entityPath(ctx(svc, { can: () => false }), 'customer', 'c1'), null);
    assert.equal(entityPath(ctx(svc, { can: (p) => p !== 'orders.read' }), 'order', 'o1'), null);
    assert.equal(entityPath(ctx(svc, { routeOn: (r) => r !== 'leads' }), 'lead', 'l1'), null);
    assert.equal(entityPath(ctx(['(app)', 'admin', 'menu']), 'transfer', 'x1'), null);
    assert.equal(entityPath(ctx(['(app)', 'depo', 'jobs']), 'customer', 'c1'), null);
    assert.equal(entityPath(ctx(['(app)', 'musteri', 'services']), 'service', 's1'), null);
    assert.equal(entityPath(ctx(svc), 'vehicle', 'v1'), null);
    assert.equal(entityPath(ctx(svc), 'user', 'u1'), null);
    assert.equal(entityPath(ctx(svc), 'order', ''), null);
    assert.equal(entityPath(ctx(svc), 'order', null), null);
  });

  it('opens organizations in the network screens of the center and admins only', () => {
    assert.equal(entityPath(ctx(['(app)', 'admin', 'menu']), 'organization', 'g1'), '/admin/network/g1');
    assert.equal(entityPath(ctx(['(app)', 'bayi', 'menu'], { orgType: 'center' }), 'organization', 'g1'), '/bayi/menu/network/g1');
    assert.equal(entityPath(ctx(['(app)', 'bayi', 'menu'], { orgType: 'dealer' }), 'organization', 'g1'), null);
    assert.equal(entityPath(ctx(['(app)', 'admin', 'menu'], { can: () => false }), 'organization', 'g1'), null);
  });

  it('opens products and stock units in the catalog or stock screens', () => {
    assert.equal(entityPath(ctx(['(app)', 'bayi', 'menu', 'orders']), 'product', 'p1'), '/bayi/menu/catalog/products/p1');
    const noCatalog = ctx(['(app)', 'bayi', 'menu'], { can: (p) => p !== 'catalog.read' && ALL.includes(p) });
    assert.equal(entityPath(noCatalog, 'product', 'p1'), '/bayi/stock/p1');
    assert.equal(entityPath(ctx(['(app)', 'admin', 'menu', 'orders']), 'product', 'p1'), '/admin/menu/stock/p1');
    assert.equal(entityPath(ctx(['(app)', 'depo', 'menu', 'orders']), 'stock_unit', 'B-1'), '/depo/stock/unit/B-1');
    assert.equal(entityPath(ctx(['(app)', 'bayi', 'menu', 'stock']), 'stock_unit', 'B-1'), '/bayi/menu/stock/unit/B-1');
    assert.equal(entityPath(ctx(['(app)', 'depo', 'menu'], { can: () => false }), 'stock_unit', 'B-1'), null);
  });

  it('keeps the old per-module hooks on the same routes', () => {
    assert.equal(menuRoutePath('bayi', 'lead', 'l1'), '/bayi/menu/leads/l1');
    assert.equal(menuRoutePath('admin', 'warranty_claim', 'w1'), '/admin/menu/warranty_claims/w1');
    assert.equal(menuRoutePath('bayi', 'cari', 'k1'), '/bayi/menu/accounting/k1');
    assert.equal(menuRoutePath('admin', 'invoice', 'e1'), '/admin/menu/einvoices/e1');
    assert.equal(menuRoutePath('depo', 'order', 'o1'), '/depo/menu/orders/o1');
  });

  it('has an icon for every kind', () => {
    for (const k of ENTITY_KINDS) assert.ok(KIND_ICONS[k], k);
  });
});
