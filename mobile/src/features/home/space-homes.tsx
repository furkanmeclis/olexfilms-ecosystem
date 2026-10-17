import {
  ArrowLeftRight,
  CalendarDays,
  CarFront,
  ClipboardCheck,
  Gauge,
  PackageCheck,
  PackagePlus,
  ScanLine,
  UserRound,
} from '@/ui/icons';

import { HomeScreen } from './home-screen';

export function BayiHome() {
  return (
    <HomeScreen
      actions={[
        { key: 'vehicle_intake', icon: CarFront, href: '/bayi/services' },
        { key: 'measure', icon: Gauge, href: '/bayi/measure' },
        { key: 'customer', icon: UserRound, href: '/search' },
        { key: 'appointment', icon: CalendarDays, href: '/bayi/menu' },
      ]}
    />
  );
}

export function DepoHome() {
  return (
    <HomeScreen
      actions={[
        { key: 'prepare', icon: PackageCheck, href: '/depo/jobs' },
        { key: 'stock_entry', icon: PackagePlus, href: '/depo/scan' },
        { key: 'transfer', icon: ArrowLeftRight, href: '/depo/jobs' },
        { key: 'count', icon: ScanLine, href: '/depo/jobs' },
      ]}
    />
  );
}

export function AdminHome() {
  return (
    <HomeScreen
      actions={[
        { key: 'customer', icon: UserRound, href: '/search' },
        { key: 'appointment', icon: ClipboardCheck, href: '/admin/approvals' },
      ]}
    />
  );
}
