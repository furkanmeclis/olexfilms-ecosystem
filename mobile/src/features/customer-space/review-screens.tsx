import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, RefreshControl, ScrollView, Switch, TextInput, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { errorMessage } from '@/core/i18n';
import { formatDate } from '@/features/services/lib';
import { DetailSkeleton, ErrorState, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { retryPortal, useFleetRoles } from './lib';

type S = components['schemas'];
type ReviewState = S['PortalServiceReview'];
type Question = S['ReviewFormQuestion'];
type Product = S['ReviewProduct'];
type AnswerInput = S['ReviewAnswerInput'];

const COMMENT_MAX = 2000;
const STARS = [1, 2, 3, 4, 5] as const;

type Row = { key: string; question: Question; product: Product | null };
type Values = Record<string, number | string>;

/** One row per question; a product question repeats for each product of the service. */
function formRows(questions: readonly Question[], products: readonly Product[]): Row[] {
  const rows: Row[] = [];
  for (const question of [...questions].sort((a, b) => a.sort_order - b.sort_order)) {
    if (question.target === 'product') {
      for (const product of products) rows.push({ key: `${question.uuid}:${product.uuid}`, question, product });
    } else {
      rows.push({ key: question.uuid, question, product: null });
    }
  }
  return rows;
}

function isAnswered(row: Row, value: number | string | undefined) {
  if (row.question.question_type === 'rating_1_5') return typeof value === 'number' && value >= 1 && value <= 5;
  return typeof value === 'string' && value.trim() !== '';
}

function toAnswers(rows: readonly Row[], values: Values): AnswerInput[] {
  const out: AnswerInput[] = [];
  for (const row of rows) {
    const value = values[row.key];
    if (!isAnswered(row, value)) continue;
    const answer: AnswerInput = { question_uuid: row.question.uuid };
    if (row.product) answer.product_uuid = row.product.uuid;
    if (row.question.question_type === 'rating_1_5') answer.rating = value as number;
    else answer.text = (value as string).trim();
    out.push(answer);
  }
  return out;
}

/** 1-5 stars; read-only when `onChange` is missing. */
function Stars({ label, value, onChange }: { label: string; value: number; onChange?: (v: number) => void }) {
  const { t } = useTranslation('customer');
  return (
    <View className="gap-1.5">
      <Text variant="callout" className="font-medium">
        {label}
      </Text>
      <View className="flex-row gap-1" accessibilityLabel={label} accessibilityValue={{ text: t('review_stars', { count: value }) }}>
        {STARS.map((n) => {
          const icon = <Icon name="star" tone={n <= value ? 'primary' : 'muted-foreground'} size={30} />;
          return onChange ? (
            <Pressable
              key={n}
              accessibilityRole="button"
              accessibilityLabel={t('review_stars', { count: n })}
              accessibilityState={{ selected: n === value }}
              hitSlop={4}
              onPress={() => onChange(n)}
              className={cn('p-0.5', n > value && 'opacity-40')}>
              {icon}
            </Pressable>
          ) : (
            <View key={n} className={cn('p-0.5', n > value && 'opacity-40')}>
              {icon}
            </View>
          );
        })}
      </View>
    </View>
  );
}

function TextBox({ value, onChange, label }: { value: string; onChange: (v: string) => void; label: string }) {
  return (
    <TextInput
      accessibilityLabel={label}
      placeholder={label}
      placeholderTextColorClassName="accent-muted-foreground"
      value={value}
      onChangeText={onChange}
      multiline
      maxLength={COMMENT_MAX}
      textAlignVertical="top"
      className="min-h-24 rounded-lg border border-border bg-card px-4 py-3 text-[17px] text-foreground focus:border-ring"
    />
  );
}

function rowLabel(row: Row) {
  const text = row.question.text || row.question.question_key;
  return row.product ? `${text} · ${row.product.name}` : text;
}

function ReviewForm({ uuid, rows }: { uuid: string; rows: Row[] }) {
  const { t } = useTranslation('customer');
  const qc = useQueryClient();
  const [platform, setPlatform] = useState(0);
  const [product, setProduct] = useState(0);
  const [comment, setComment] = useState('');
  const [anonymous, setAnonymous] = useState(false);
  const [values, setValues] = useState<Values>({});
  const missing = platform < 1 || product < 1 || rows.some((r) => r.question.is_required && !isAnswered(r, values[r.key]));

  const submit = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST('/v1/portal/services/{uuid}/review', {
          params: { path: { uuid } },
          body: {
            platform_rating: platform,
            product_rating: product,
            comment: comment.trim() || null,
            is_anonymous: anonymous,
            source: 'portal',
            // The legacy body (two ratings) when the brand has no admin question.
            ...(rows.length ? { answers: toAnswers(rows, values) } : {}),
          },
        }),
      ),
    onSuccess: (next) => qc.setQueryData(['portal', 'services', 'review', uuid], next),
    onError: (e) => {
      if (isApiError(e) && e.status === 409) void qc.invalidateQueries({ queryKey: ['portal', 'services', 'review', uuid] });
      Alert.alert(errorMessage(isApiError(e) ? e.code : undefined));
    },
  });

  return (
    <View className="gap-4 rounded-lg border border-border bg-card p-4">
      <Stars label={t('review_platform')} value={platform} onChange={setPlatform} />
      <Stars label={t('review_product')} value={product} onChange={setProduct} />
      {rows.map((row) =>
        row.question.question_type === 'rating_1_5' ? (
          <Stars
            key={row.key}
            label={rowLabel(row) + (row.question.is_required ? ' *' : '')}
            value={typeof values[row.key] === 'number' ? (values[row.key] as number) : 0}
            onChange={(v) => setValues((p) => ({ ...p, [row.key]: v }))}
          />
        ) : (
          <View key={row.key} className="gap-1.5">
            <Text variant="callout" className="font-medium">
              {rowLabel(row) + (row.question.is_required ? ' *' : '')}
            </Text>
            <TextBox
              label={rowLabel(row)}
              value={typeof values[row.key] === 'string' ? (values[row.key] as string) : ''}
              onChange={(v) => setValues((p) => ({ ...p, [row.key]: v }))}
            />
          </View>
        ),
      )}
      <View className="gap-1.5">
        <Text variant="callout" className="font-medium">
          {t('review_comment')}
        </Text>
        <TextBox label={t('review_comment_placeholder')} value={comment} onChange={setComment} />
      </View>
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="callout" className="flex-1">
          {t('review_anonymous')}
        </Text>
        <Switch value={anonymous} onValueChange={setAnonymous} />
      </View>
      <Button title={t('review_submit')} size="lg" loading={submit.isPending} disabled={missing} onPress={() => submit.mutate()} />
    </View>
  );
}

function SubmittedReview({ state, rows }: { state: ReviewState; rows: Row[] }) {
  const { t, i18n } = useTranslation('customer');
  const r = state.review!;
  const answerOf = (row: Row) =>
    r.answers.find((a) => a.question_uuid === row.question.uuid && (a.product_uuid ?? null) === (row.product?.uuid ?? null));
  return (
    <View className="gap-4 rounded-lg border border-border bg-card p-4">
      <View className="flex-row items-center gap-2">
        <Icon name="check" tone="primary" size={18} weight="semibold" />
        <Text variant="callout" className="flex-1 font-semibold">
          {t('review_sent', { date: formatDate(r.created_at, i18n.language) })}
        </Text>
      </View>
      <Stars label={t('review_platform')} value={r.platform_rating} />
      <Stars label={t('review_product')} value={r.product_rating} />
      {rows
        .filter((row) => answerOf(row))
        .map((row) =>
          row.question.question_type === 'rating_1_5' ? (
            <Stars key={row.key} label={rowLabel(row)} value={answerOf(row)?.rating ?? 0} />
          ) : (
            <View key={row.key} className="gap-0.5">
              <Text variant="callout" className="font-medium">
                {rowLabel(row)}
              </Text>
              <Text variant="callout" tone="muted">
                {answerOf(row)?.text}
              </Text>
            </View>
          ),
        )}
      {r.comment ? (
        <View className="gap-0.5">
          <Text variant="callout" className="font-medium">
            {t('review_comment')}
          </Text>
          <Text variant="callout" tone="muted" selectable>
            {r.comment}
          </Text>
        </View>
      ) : null}
      {r.is_anonymous ? (
        <Text variant="footnote" tone="muted">
          {t('review_anonymous_sent')}
        </Text>
      ) : null}
    </View>
  );
}

/** Review of a completed service (GET/POST /v1/portal/services/{uuid}/review): stars and comment, or the sent review. */
export function CustomerServiceReviewScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('customer');
  const { readOnly } = useFleetRoles();
  const q = useQuery({
    queryKey: ['portal', 'services', 'review', uuid],
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/services/{uuid}/review', { params: { path: { uuid } } })),
  });
  const state = q.data;
  const rows = useMemo(() => formRows(state?.questions ?? [], state?.products ?? []), [state]);
  const google = state?.google_business_url;

  return (
    <>
      <Stack.Screen options={{ title: t('review'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        automaticallyAdjustKeyboardInsets
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {!state ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            {state.review ? (
              <SubmittedReview state={state} rows={rows} />
            ) : state.can_review && !readOnly ? (
              <ReviewForm uuid={uuid} rows={rows} />
            ) : (
              <View className="rounded-lg border border-border bg-card p-4">
                <Text variant="callout" tone="muted">
                  {readOnly ? t('review_read_only') : t('review_not_available')}
                </Text>
              </View>
            )}
            {google ? (
              <Section title={t('review_google_title')}>
                <Pressable
                  accessibilityRole="link"
                  onPress={() => void WebBrowser.openBrowserAsync(google)}
                  className="min-h-12 flex-row items-center gap-3 px-4 py-3 active:bg-muted">
                  <Text variant="callout" tone="primary" className="flex-1">
                    {t('review_google')}
                  </Text>
                  <Icon name="externalLink" tone="primary" size={18} />
                </Pressable>
              </Section>
            ) : null}
          </>
        )}
      </ScrollView>
    </>
  );
}
