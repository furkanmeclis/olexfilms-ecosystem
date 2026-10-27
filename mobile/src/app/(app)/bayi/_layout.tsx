import { NativeTabs } from 'expo-router/native-tabs';
import { useTranslation } from 'react-i18next';

import { useModule } from '@/core/modules';
import { useCan } from '@/core/spaces';
import { useTabTint } from '@/features/shell/tab-tint';

/**
 * Dealer space. Measuring sits in the tab bar for everyone who can create a
 * service (decision 2026-10-16); others get Stock in that slot instead.
 */
export default function BayiTabs() {
  const tint = useTabTint();
  const { t } = useTranslation('nav');
  const measures = useCan('services.write');
  // Roles without services or stock (center/dealer accounting) lose those tabs instead of a 403.
  const services = useCan('services.read');
  const canStock = useCan('stock.read');
  // A switched-off module loses its tab (module flags, TEC-1023).
  const measurements = useModule('measurements');
  const stock = useModule('stock');
  return (
    <NativeTabs tintColor={tint}>
      <NativeTabs.Trigger name="(home)">
        <NativeTabs.Trigger.Icon sf="house.fill" md="home" />
        <NativeTabs.Trigger.Label>{t('home')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="services" hidden={!services}>
        <NativeTabs.Trigger.Icon sf="wrench.and.screwdriver.fill" md="build" />
        <NativeTabs.Trigger.Label>{t('services')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="scan">
        <NativeTabs.Trigger.Icon sf="qrcode.viewfinder" md="qr_code_scanner" />
        <NativeTabs.Trigger.Label>{t('scan')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="measure" hidden={!measures || !measurements}>
        <NativeTabs.Trigger.Icon sf="gauge.with.needle.fill" md="speed" />
        <NativeTabs.Trigger.Label>{t('measure')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="stock" hidden={measures || !stock || !canStock}>
        <NativeTabs.Trigger.Icon sf="shippingbox.fill" md="inventory_2" />
        <NativeTabs.Trigger.Label>{t('stock')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="menu">
        <NativeTabs.Trigger.Icon sf="line.3.horizontal" md="menu" />
        <NativeTabs.Trigger.Label>{t('menu')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
    </NativeTabs>
  );
}
