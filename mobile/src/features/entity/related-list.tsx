import { useQuery } from '@tanstack/react-query';
import { router } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, View } from 'react-native';

import { List, ListRow, SectionHeader } from '@/ui/list';
import { Text } from '@/ui/text';
import type { IconName } from '@/ui/icons';

import { type EntityKind, KIND_ICONS } from './href';
import { useEntityHrefs } from './use-entity-href';

export const RELATED_LIMIT = 5;

export type RelatedItem = { uuid: string; title: string; subtitle?: string | null };

/**
 * Reverse relation section of a detail screen (TEC-1219): the newest records
 * of one kind that point at this record (a dealer's orders, a customer's
 * warranties ...). One small list request (`limit` 5) with the backend's
 * reverse filter; each row opens its record when the viewer may, and the
 * header shows the total. Hidden while `enabled` is false (permission or
 * module), so a closed module draws nothing.
 */
export function RelatedList({
  kind,
  title,
  queryKey,
  fetch,
  enabled = true,
}: {
  kind: EntityKind;
  title: string;
  queryKey: readonly unknown[];
  fetch: (limit: number) => Promise<{ items: RelatedItem[]; total: number }>;
  enabled?: boolean;
}) {
  const { t } = useTranslation('common');
  const hrefOf = useEntityHrefs();
  const q = useQuery({ queryKey: [...queryKey, 'related', RELATED_LIMIT], queryFn: () => fetch(RELATED_LIMIT), enabled });
  if (!enabled) return null;
  const items = q.data?.items ?? [];
  const total = q.data?.total;
  const more = total !== undefined ? total - items.length : 0;

  return (
    <View>
      <SectionHeader title={total !== undefined ? `${title} (${total})` : title} />
      {q.isPending ? (
        <ActivityIndicator className="py-3" />
      ) : q.isError ? (
        <Text variant="footnote" tone="muted" className="px-4">
          {t('error_generic')}
        </Text>
      ) : items.length === 0 ? (
        <Text variant="footnote" tone="muted" className="px-4">
          {t('related_empty')}
        </Text>
      ) : (
        <List>
          {items.map((it, i) => {
            const href = hrefOf(kind, it.uuid);
            return (
              <ListRow
                key={it.uuid}
                icon={KIND_ICONS[kind] as IconName}
                title={it.title}
                subtitle={it.subtitle ?? undefined}
                onPress={href ? () => router.push(href) : undefined}
                last={i === items.length - 1 && more <= 0}
              />
            );
          })}
          {more > 0 ? (
            <Text variant="footnote" tone="muted" className="px-4 py-2.5">
              {t('related_more', { count: more })}
            </Text>
          ) : null}
        </List>
      )}
    </View>
  );
}
