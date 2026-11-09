import { usePathname } from 'expo-router';

import { useSession } from '@/core/auth/session';
import { pricesVisible } from '@/core/price-visibility';
import { kindsOf } from '@/core/spaces';

/** Whether this screen may show prices, amounts and totals (TEC-1051). */
export function usePricesVisible(): boolean {
  const pathname = usePathname();
  const me = useSession((s) => s.me);
  return pricesVisible({
    permissions: me?.permissions ?? [],
    superAdmin: !!me?.user.is_super_admin,
    primarySpace: kindsOf(me)[0],
    pathname,
  });
}
