import { TabStack } from '@/features/shell/tab-stack';

/** Customer appointments tab: every screen is the `appointments` module (closed module → no screen). */
export default function CustomerAppointmentsStack() {
  return <TabStack module="appointments" />;
}
