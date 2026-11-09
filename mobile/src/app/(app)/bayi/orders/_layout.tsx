import { TabStack } from '@/features/shell/tab-stack';

/** Orders tab of the center sales staff (TEC-1068); an order opened from home keeps the list under it. */
export const unstable_settings = { anchor: 'index' };

export default function OrdersTab() {
  return <TabStack module="orders" />;
}
