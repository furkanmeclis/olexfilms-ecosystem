import { type Href, router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { CalendarDays, CarFront, Info, type LucideIcon, ShieldCheck, UserRound, Wrench } from '@/ui/icons';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

/**
 * Customer space. Customer accounts cannot sign in on mobile yet (the mobile
 * API answers 403 NO_PANEL_ACCESS, mobile/README.md "Backend gaps"), so these
 * screens make no API calls: they show their empty states and a short note.
 */
function SoonNote() {
  const { t } = useTranslation('customer');
  return (
    <View className="flex-row items-start gap-3 rounded-lg border border-border bg-secondary px-4 py-3">
      <Icon as={Info} tone="primary" size={18} />
      <Text variant="footnote" className="flex-1">
        {t('soon_note')}
      </Text>
    </View>
  );
}

function Section({ icon, title, empty, body }: { icon: LucideIcon; title: string; empty: string; body: string }) {
  return (
    <>
      <SectionHeader title={title} />
      <View className="rounded-lg border border-border bg-card">
        <EmptyState icon={icon} title={empty} body={body} />
      </View>
    </>
  );
}

function Page({ titleKey, children }: { titleKey: string; children: React.ReactNode }) {
  const { t } = useTranslation('customer');
  return (
    <>
      <Stack.Screen options={{ title: t(titleKey) }} />
      <Screen>
        <SoonNote />
        {children}
      </Screen>
    </>
  );
}

export function CustomerHomeScreen() {
  const { t } = useTranslation('customer');
  const name = useSession((s) => s.me?.user.name);
  return (
    <>
      <Stack.Screen options={{ title: name ? t('hello', { name }) : t('home') }} />
      <Screen>
        <SoonNote />
        <List>
          <ListRow title={t('vehicles')} leading={<Icon as={CarFront} tone="primary" size={20} />} onPress={() => router.push('/musteri/vehicles' as Href)} />
          <ListRow title={t('services')} leading={<Icon as={Wrench} tone="primary" size={20} />} onPress={() => router.push('/musteri/services' as Href)} />
          <ListRow title={t('warranties')} leading={<Icon as={ShieldCheck} tone="primary" size={20} />} onPress={() => router.push('/musteri/warranties')} />
          <ListRow title={t('appointments')} leading={<Icon as={CalendarDays} tone="primary" size={20} />} onPress={() => router.push('/musteri/appointments')} last />
        </List>
      </Screen>
    </>
  );
}

export function CustomerVehiclesScreen() {
  const { t } = useTranslation('customer');
  return (
    <Page titleKey="vehicles">
      <Section icon={CarFront} title={t('vehicles')} empty={t('vehicles_empty')} body={t('vehicles_empty_body')} />
    </Page>
  );
}

export function CustomerServicesScreen() {
  const { t } = useTranslation('customer');
  return (
    <Page titleKey="services">
      <Section icon={Wrench} title={t('services')} empty={t('services_empty')} body={t('services_empty_body')} />
    </Page>
  );
}

export function CustomerWarrantiesScreen() {
  const { t } = useTranslation('customer');
  return (
    <Page titleKey="warranties">
      <Section icon={ShieldCheck} title={t('warranties')} empty={t('warranties_empty')} body={t('warranties_empty_body')} />
    </Page>
  );
}

export function CustomerAppointmentsScreen() {
  const { t } = useTranslation('customer');
  return (
    <Page titleKey="appointments">
      <Section icon={CalendarDays} title={t('appointments')} empty={t('appointments_empty')} body={t('appointments_empty_body')} />
    </Page>
  );
}

export function CustomerProfileScreen() {
  const { t } = useTranslation('customer');
  const user = useSession((s) => s.me?.user);
  return (
    <Page titleKey="profile">
      <View className="items-center gap-2 py-4">
        <View className="size-16 items-center justify-center rounded-full bg-secondary">
          <Icon as={UserRound} tone="primary" size={30} />
        </View>
        <Text variant="headline">{[user?.name, user?.surname].filter(Boolean).join(' ') || t('profile')}</Text>
        {user?.email ? (
          <Text variant="footnote" tone="muted">
            {user.email}
          </Text>
        ) : null}
      </View>
      <List>
        <ListRow title={t('settings')} onPress={() => router.push('/settings')} />
        <ListRow title={t('sign_out')} tone="danger" onPress={() => void useSession.getState().signOut()} last />
      </List>
    </Page>
  );
}
