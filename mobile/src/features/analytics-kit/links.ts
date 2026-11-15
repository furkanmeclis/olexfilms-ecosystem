/**
 * Detail route of a report row link (`_link: { kind, uuid }`, TEC-1210) in
 * the space the user is in. Routes stay inside the current tab's stack (the
 * menu), so Back returns to the report. Kinds a space has no screen for give
 * null and the row is not tappable. Pure, so node --test runs it.
 */

export type Space = 'bayi' | 'admin' | 'depo' | string;

export function linkPath(space: Space, kind: string, uuid: string | null | undefined, report?: string | null): string | null {
  if (kind === 'report') {
    // A KPI's detail report (TEC-1210 `{ kind: report, report: key }`); the warehouse space has no report screens.
    if (!report || !/^[\w.]+$/.test(report) || (space !== 'bayi' && space !== 'admin')) return null;
    return `/${space}/menu/reports/${encodeURIComponent(report)}`;
  }
  if (!uuid || !/^[\w-]+$/.test(uuid)) return null;
  if (space === 'depo') return kind === 'order' ? `/depo/menu/orders/${uuid}` : null;
  if (space !== 'bayi' && space !== 'admin') return null;
  switch (kind) {
    case 'order':
      return `/${space}/menu/orders/${uuid}`;
    case 'customer':
      return `/${space}/menu/customers/${uuid}`;
    case 'service':
      return `/${space}/menu/customers/services/${uuid}`;
    case 'warranty':
      return `/${space}/menu/customers/warranties/${uuid}`;
    case 'measurement':
      return `/${space}/menu/measurements/${uuid}`;
    case 'organization':
      return space === 'admin' ? `/admin/network/${uuid}` : `/bayi/menu/network/${uuid}`;
    case 'product':
      return space === 'bayi' ? `/bayi/menu/catalog/products/${uuid}` : null;
    default:
      return null;
  }
}
