import { TabStack } from '@/features/shell/tab-stack';

/** Customer fleet tab: every screen is the `fleet` module (closed module → no screen). */
export default function CustomerFleetStack() {
  return <TabStack module="fleet" />;
}
