import { useQuery } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { formatDate, formatDateTime } from '@/features/services/lib';
import { DetailSkeleton, ErrorState, InfoRow, Section } from '@/features/services/ui';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { useAccountingAccess } from './hooks';
import { einvoiceTimeline, einvoiceTone, formatMoney, hasWebOnlyActions, validationTone } from './lib';

/**
 * One e-invoice, read only: summary, buyer, lines, VAT, validation messages
 * and status history. Archive and void need a step-up and stay on the web,
 * as do the XML/PDF downloads and the HTML preview.
 */
export function EinvoiceDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('accounting');
  const access = useAccountingAccess();

  const q = useQuery({
    queryKey: ['einvoices', 'detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/einvoices/{uuid}', { params: { path: { uuid } } })),
  });
  const inv = q.data;
  const money = (v: string) => (inv ? formatMoney(v, inv.currency, i18n.language) : v);

  return (
    <>
      <Stack.Screen options={{ title: inv?.number ?? t('einvoice.detail_title'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {!inv ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <View className="gap-2 rounded-lg border border-border bg-card p-4">
              <Text variant="title" selectable className="font-mono">
                {inv.number ?? t('einvoice.draft_number')}
              </Text>
              <Text variant="callout">{inv.buyer_organization?.name ?? inv.buyer.name}</Text>
              <View className="flex-row flex-wrap gap-2">
                <StatusPill label={t(`einvoice.status.${inv.status}`)} tone={einvoiceTone(inv.status)} />
                <StatusPill label={t(`einvoice.validation.${inv.validation_status}`)} tone={validationTone(inv.validation_status)} />
              </View>
              <Text variant="title" className="tabular-nums">
                {money(inv.payable)}
              </Text>
            </View>

            {inv.error ? (
              <View className="flex-row gap-2 rounded-lg bg-destructive/10 p-3">
                <Icon name="alert" tone="destructive" size={18} />
                <Text variant="footnote" tone="danger" selectable className="flex-1">
                  {inv.error}
                </Text>
              </View>
            ) : null}

            {access.canManageEinvoices && hasWebOnlyActions(inv) ? (
              <View className="flex-row gap-2 rounded-lg bg-secondary p-3">
                <Icon name="info" tone="muted-foreground" size={18} />
                <Text variant="footnote" tone="muted" className="flex-1">
                  {t('einvoice.web_only')}
                </Text>
              </View>
            ) : null}

            <Section title={t('einvoice.summary')}>
              <InfoRow label={t('einvoice.ettn')} value={inv.uuid} mono />
              <InfoRow label={t('einvoice.profile_label')} value={t(`einvoice.profile.${inv.profile}`)} />
              <InfoRow label={t('einvoice.issue_date')} value={formatDate(inv.issue_date, i18n.language)} />
              <InfoRow label={t('einvoice.source')} value={t(`einvoice.source_type.${inv.source_type}`)} />
              <InfoRow label={t('einvoice.tax_exclusive')} value={money(inv.tax_exclusive)} />
              <InfoRow label={t('einvoice.tax_total')} value={money(inv.tax_total)} />
              <InfoRow label={t('einvoice.payable')} value={money(inv.payable)} last />
            </Section>

            <Section title={t('einvoice.buyer')}>
              <InfoRow label={t('einvoice.buyer_name')} value={inv.buyer.name} />
              <InfoRow label="VKN" value={inv.buyer.vkn} mono />
              <InfoRow label="TCKN" value={inv.buyer.tckn} mono />
              <InfoRow label={t('einvoice.tax_office')} value={inv.buyer.tax_office} />
              <InfoRow
                label={t('einvoice.address')}
                value={[inv.buyer.street, inv.buyer.district, inv.buyer.city].filter(Boolean).join(', ')}
                last
              />
            </Section>

            {inv.lines.length > 0 ? (
              <Section title={t('einvoice.lines')}>
                {inv.lines.map((l, i) => (
                  <View key={i} className={`gap-0.5 px-4 py-2.5 ${i < inv.lines.length - 1 ? 'border-b border-border' : ''}`}>
                    <View className="flex-row items-start justify-between gap-3">
                      <Text variant="callout" className="flex-1">
                        {l.name}
                      </Text>
                      <Text variant="callout" className="tabular-nums">
                        {money(l.line_extension)}
                      </Text>
                    </View>
                    <Text variant="footnote" tone="muted">
                      {t('einvoice.line_meta', { quantity: l.quantity, price: money(l.unit_price), vat: l.vat_rate })}
                    </Text>
                  </View>
                ))}
              </Section>
            ) : null}

            {inv.tax_breakdown.length > 0 ? (
              <Section title={t('einvoice.taxes')}>
                {inv.tax_breakdown.map((x, i) => (
                  <InfoRow
                    key={x.percent}
                    label={t('einvoice.vat_rate', { rate: x.percent })}
                    value={money(x.tax_amount)}
                    last={i === inv.tax_breakdown.length - 1}
                  />
                ))}
              </Section>
            ) : null}

            <Section title={t('einvoice.validation_messages')}>
              {inv.validation_messages.length === 0 ? (
                <Text variant="callout" tone="muted" className="px-4 py-3">
                  {t('einvoice.no_messages')}
                </Text>
              ) : (
                inv.validation_messages.map((m, i) => (
                  <Text
                    key={i}
                    variant="footnote"
                    selectable
                    className={`px-4 py-2.5 ${i < inv.validation_messages.length - 1 ? 'border-b border-border' : ''}`}>
                    {m}
                  </Text>
                ))
              )}
            </Section>

            <Section title={t('einvoice.timeline_title')}>
              {einvoiceTimeline(inv).map((s, i, all) => (
                <View key={s.id} className={`gap-0.5 px-4 py-2.5 ${i < all.length - 1 ? 'border-b border-border' : ''}`}>
                  <Text variant="callout">{t(`einvoice.timeline.${s.id}`)}</Text>
                  <Text variant="footnote" tone="muted">
                    {[s.id === 'archived' ? formatDate(s.at, i18n.language) : formatDateTime(s.at, i18n.language), s.note]
                      .filter(Boolean)
                      .join(' · ')}
                  </Text>
                </View>
              ))}
            </Section>
          </>
        )}
      </ScrollView>
    </>
  );
}
