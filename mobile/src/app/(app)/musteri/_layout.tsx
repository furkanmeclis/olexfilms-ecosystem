import { NativeTabs } from 'expo-router/native-tabs';
import { useTranslation } from 'react-i18next';

/**
 * Customer space. Not reachable yet: the mobile API refuses customer-only
 * accounts (403 NO_PANEL_ACCESS); see mobile/README.md "Backend gaps".
 */
export default function MusteriTabs() {
  const { t } = useTranslation('nav');
  return (
    <NativeTabs>
      <NativeTabs.Trigger name="(home)">
        <NativeTabs.Trigger.Icon sf="car.fill" md="directions_car" />
        <NativeTabs.Trigger.Label>{t('vehicles')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="warranties">
        <NativeTabs.Trigger.Icon sf="checkmark.shield.fill" md="verified_user" />
        <NativeTabs.Trigger.Label>{t('warranties')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="appointments">
        <NativeTabs.Trigger.Icon sf="calendar" md="calendar_month" />
        <NativeTabs.Trigger.Label>{t('appointments')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="profile">
        <NativeTabs.Trigger.Icon sf="person.crop.circle.fill" md="account_circle" />
        <NativeTabs.Trigger.Label>{t('profile')}</NativeTabs.Trigger.Label>
      </NativeTabs.Trigger>
    </NativeTabs>
  );
}
