import { NativeTabs } from 'expo-router/native-tabs';
import { useTranslation } from 'react-i18next';
import { useTabTint } from '@/features/shell/tab-tint';

/**
 * Admin space, the same tabs for the center and distributors: the center sees
 * the whole network, a distributor its own subtree (scope comes from the API).
 */
export default function AdminTabs() {
  const tint = useTabTint();
  const { t } = useTranslation('nav');
  return (
    <NativeTabs tintColor={tint}>
      <NativeTabs.Trigger name="(home)">
        <NativeTabs.Trigger.Icon sf="house.fill" md="home" />
        <NativeTabs.Trigger.Label>{t('home')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="approvals">
        <NativeTabs.Trigger.Icon sf="checkmark.seal.fill" md="task_alt" />
        <NativeTabs.Trigger.Label>{t('approvals')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="scan">
        <NativeTabs.Trigger.Icon sf="qrcode.viewfinder" md="qr_code_scanner" />
        <NativeTabs.Trigger.Label>{t('scan')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="network">
        <NativeTabs.Trigger.Icon sf="point.3.connected.trianglepath.dotted" md="hub" />
        <NativeTabs.Trigger.Label>{t('network')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="menu">
        <NativeTabs.Trigger.Icon sf="line.3.horizontal" md="menu" />
        <NativeTabs.Trigger.Label>{t('menu')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
    </NativeTabs>
  );
}
