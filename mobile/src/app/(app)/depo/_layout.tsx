import { NativeTabs } from 'expo-router/native-tabs';
import { useTranslation } from 'react-i18next';
import { useTabTint } from '@/features/shell/tab-tint';

/** Warehouse space: the whole warehouse runs here, the web is not needed (decision 2026-10-16). */
export default function DepoTabs() {
  const tint = useTabTint();
  const { t } = useTranslation('nav');
  return (
    <NativeTabs tintColor={tint}>
      <NativeTabs.Trigger name="(home)">
        <NativeTabs.Trigger.Icon sf="house.fill" md="home" />
        <NativeTabs.Trigger.Label>{t('home')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="jobs">
        <NativeTabs.Trigger.Icon sf="tray.full.fill" md="assignment" />
        <NativeTabs.Trigger.Label>{t('jobs')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="scan">
        <NativeTabs.Trigger.Icon sf="qrcode.viewfinder" md="qr_code_scanner" />
        <NativeTabs.Trigger.Label>{t('scan')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="stock">
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
