import { useMutation, useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { formatDateTime } from '@/features/services/lib';
import { Chips, ListBody, ListFooter } from '@/features/services/ui';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { PAGE_SIZE, type PortalAppointment, useFleetRoles, usePortalPages } from './lib';
import { AppointmentRow } from './parts';

type Period = 'upcoming' | 'past';

/**
 * The customer's appointments (GET /v1/portal/appointments) with cancel and
 * a "new appointment" entry (booking-screens). A fleet session cannot book.
 */
export function CustomerAppointmentsScreen() {
  const { t, i18n } = useTranslation('customer');
  const qc = useQueryClient();
  const { readOnly } = useFleetRoles();
  const [period, setPeriod] = useState<Period>('upcoming');
  const list = usePortalPages(['portal', 'appointments', 'list', { period }], (offset) =>
    unwrap(api.GET('/v1/portal/appointments', { params: { query: { period, limit: PAGE_SIZE, offset } } })),
  );

  const cancel = useMutation({
    mutationFn: (uuid: string) =>
      unwrap(api.POST('/v1/portal/appointments/{uuid}/cancel', { params: { path: { uuid } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['portal', 'appointments'] });
    },
    onError: (e) => {
      const msg = isApiError(e, 'APPOINTMENT_CANCEL_WINDOW_CLOSED')
        ? t('appointment_cancel_window_closed')
        : errorMessage(isApiError(e) ? e.code : undefined);
      Alert.alert(t('appointment_cancel_failed'), msg);
    },
  });

  const confirmCancel = (a: PortalAppointment) => {
    Alert.alert(
      t('appointment_cancel_title'),
      [formatDateTime(a.starts_at, i18n.language), a.dealer_name].filter(Boolean).join(' · '),
      [
        { text: t('keep'), style: 'cancel' },
        { text: t('appointment_cancel'), style: 'destructive', onPress: () => cancel.mutate(a.uuid) },
      ],
    );
  };

  const chips = [
    { value: 'upcoming' as Period, label: t('period_upcoming') },
    { value: 'past' as Period, label: t('period_past') },
  ];

  return (
    <>
      <Stack.Screen
        options={{
          title: t('appointments'),
          headerRight: readOnly
            ? undefined
            : () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('booking_new')}
                  hitSlop={8}
                  onPress={() => router.push('/musteri/appointments/new')}>
                  <Icon name="add" tone="primary" size={22} />
                </Pressable>
              ),
        }}
      />
      <FlatList
        data={list.items}
        keyExtractor={(a) => a.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2.5 px-4 pb-10"
        ListHeaderComponent={
          <View className="gap-2 pb-1 pt-2">
            <Chips options={chips} value={period} onChange={setPeriod} />
            {period === 'upcoming' && list.items.length > 0 ? (
              <Text variant="footnote" tone="muted">
                {t('appointment_cancel_hint')}
              </Text>
            ) : null}
          </View>
        }
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{
              icon: 'appointment',
              title: period === 'upcoming' ? t('appointments_empty') : t('appointments_past_empty'),
              body: period === 'upcoming' ? t('appointments_empty_body') : undefined,
              action:
                period === 'upcoming' && !readOnly
                  ? { title: t('booking_new'), onPress: () => router.push('/musteri/appointments/new') }
                  : undefined,
            }}
          />
        }
        renderItem={({ item }) => (
          <AppointmentRow
            a={item}
            onCancel={readOnly ? undefined : () => confirmCancel(item)}
            cancelling={cancel.isPending && cancel.variables === item.uuid}
          />
        )}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
        }}
        ListFooterComponent={<ListFooter loading={list.isFetchingNextPage} />}
        refreshControl={
          <RefreshControl refreshing={list.isRefetching && !list.isFetchingNextPage} onRefresh={() => void list.refetch()} />
        }
      />
    </>
  );
}
