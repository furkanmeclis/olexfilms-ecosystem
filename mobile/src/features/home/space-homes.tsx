
import { HomeScreen } from './home-screen';

export function BayiHome() {
  return (
    <HomeScreen
      actions={[
        { key: 'vehicle_intake', icon: 'vehicle', href: '/bayi/services' },
        { key: 'measure', icon: 'gauge', href: '/bayi/measure' },
        { key: 'customer', icon: 'customer', href: '/search' },
        { key: 'appointment', icon: 'appointment', href: '/bayi/menu' },
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
  return (
    <HomeScreen
      actions={[
        { key: 'customer', icon: 'customer', href: '/search' },
        { key: 'appointment', icon: 'warrantyClaim', href: '/admin/approvals' },
      ]}
    />
  );
}
