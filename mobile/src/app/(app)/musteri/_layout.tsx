import { NativeTabs } from 'expo-router/native-tabs';
import { useTranslation } from 'react-i18next';
import { useFleetRoles } from '@/features/customer-space/lib';
import { useTabTint } from '@/features/shell/tab-tint';

/**
 * Customer space: customer and fleet accounts (phone-code sign-in,
 * `me.session_kind === 'customer'`). Screens read the portal routes.
 */
export default function MusteriTabs() {
  const tint = useTabTint();
  const { t } = useTranslation('nav');
  const { fleet } = useFleetRoles();
  return (
    <NativeTabs tintColor={tint}>
      <NativeTabs.Trigger name="(home)">
        <NativeTabs.Trigger.Icon sf="house.fill" md="home" />
        <NativeTabs.Trigger.Label>{t('home')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="warranties">
        <NativeTabs.Trigger.Icon sf="checkmark.shield.fill" md="verified_user" />
        <NativeTabs.Trigger.Label>{t('warranties')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="appointments">
        <NativeTabs.Trigger.Icon sf="calendar" md="calendar_month" />
        <NativeTabs.Trigger.Label>{t('appointments')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="fleet" hidden={!fleet}>
        <NativeTabs.Trigger.Icon sf="truck.box.fill" md="local_shipping" />
        <NativeTabs.Trigger.Label>{t('fleet')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="profile">
        <NativeTabs.Trigger.Icon sf="person.crop.circle.fill" md="account_circle" />
        <NativeTabs.Trigger.Label>{t('profile')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
    </NativeTabs>
  );
}
