import { NativeTabs } from 'expo-router/native-tabs';
import { useTranslation } from 'react-i18next';

import { useCan } from '@/core/spaces';

/**
 * Dealer space. Measuring sits in the tab bar for everyone who can create a
 * service (decision 2026-10-16); others get Stock in that slot instead.
 */
export default function BayiTabs() {
  const { t } = useTranslation('nav');
  const measures = useCan('services.write');
  return (
    <NativeTabs>
      <NativeTabs.Trigger name="(home)">
        <NativeTabs.Trigger.Icon sf="house.fill" md="home" />
        <NativeTabs.Trigger.Label>{t('home')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="services">
        <NativeTabs.Trigger.Icon sf="wrench.and.screwdriver.fill" md="build" />
        <NativeTabs.Trigger.Label>{t('services')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="scan">
        <NativeTabs.Trigger.Icon sf="qrcode.viewfinder" md="qr_code_scanner" />
        <NativeTabs.Trigger.Label>{t('scan')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="measure" hidden={!measures}>
        <NativeTabs.Trigger.Icon sf="gauge.with.needle.fill" md="speed" />
        <NativeTabs.Trigger.Label>{t('measure')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="stock" hidden={measures}>
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
