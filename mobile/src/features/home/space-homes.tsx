
import { useCan } from '@/core/spaces';

import { HomeScreen, type QuickAction } from './home-screen';

export function BayiHome() {
  return (
    <HomeScreen
      actions={[
        { key: 'vehicle_intake', icon: 'vehicle', href: '/bayi/services' },
        { key: 'measure', icon: 'gauge', href: '/bayi/measure' },
        { key: 'customer', icon: 'customer', href: '/search' },
        { key: 'appointment', icon: 'appointment', href: '/bayi/menu/appointments' },
      ]}
    />
  );
}

export function DepoHome() {
  return (
    <HomeScreen
      actions={[
        { key: 'prepare', icon: 'packageCheck', href: '/depo/jobs' },
        { key: 'stock_entry', icon: 'packageIn', href: '/depo/scan' },
        { key: 'transfer', icon: 'transfer', href: '/depo/jobs' },
        { key: 'count', icon: 'scan', href: '/depo/jobs' },
      ]}
    />
  );
}

export function AdminHome() {
  // The approvals screen lists orders, module requests and reclassifications,
  // each behind its permission; the shortcut shows only when one applies
  // (center_service / center_accounting otherwise opened an empty screen).
  const orders = useCan('orders.approve');
  const modules = useCan('modules.manage');
  const reclass = useCan('stock.reclassify');
  const approvals: QuickAction[] =
    orders || modules || reclass ? [{ key: 'appointment', icon: 'warrantyClaim', href: '/admin/approvals' }] : [];
  return <HomeScreen actions={[{ key: 'customer', icon: 'customer', href: '/search' }, ...approvals]} />;
}
