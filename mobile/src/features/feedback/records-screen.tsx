import { Image } from 'expo-image';
import { Stack } from 'expo-router';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, View } from 'react-native';

import { formatDateTime } from '@/features/services/lib';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { List } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import { cn } from '@/ui/cn';

import type { FeedbackMeta } from './meta';
import { ShareZipButton } from './share-button';
import { recordFile, useFeedback } from './store';

function Row({ r, last }: { r: FeedbackMeta; last: boolean }) {
  const { t, i18n } = useTranslation('feedback');
  const thumb = recordFile(r.id, r.files.annotated ? 'annotated.png' : 'original.png').uri;

  const remove = () =>
    Alert.alert(t('delete_title'), t('delete_body'), [
      { text: t('cancel'), style: 'cancel' },
      { text: t('delete'), style: 'destructive', onPress: () => useFeedback.getState().remove(r.id) },
    ]);

  return (
    <View className={cn('flex-row items-center gap-3 px-4 py-3', !last && 'border-b border-border')}>
      <Image
        source={{ uri: thumb }}
        style={{ width: 48, height: 96, borderRadius: 6 }}
        contentFit="cover"
        recyclingKey={r.id}
        accessibilityIgnoresInvertColors
      />
      <View className="flex-1 gap-0.5">
        <Text variant="body" numberOfLines={3} tone={r.note ? 'default' : 'muted'}>
          {r.note || t('no_note')}
        </Text>
        <Text variant="footnote" tone="muted">
          {formatDateTime(r.created_at, i18n.language)}
        </Text>
        {/* Paths are code, left-to-right in every language. */}
        <Text variant="footnote" tone="muted" numberOfLines={1} className="font-mono" style={{ writingDirection: 'ltr' }}>
          {r.location.pathname}
        </Text>
        {r.files.annotated ? (
          <Text variant="footnote" tone="primary">
            {t('has_drawing')}
          </Text>
        ) : null}
      </View>
      <Pressable accessibilityRole="button" accessibilityLabel={t('delete')} hitSlop={10} onPress={remove} className="p-1 active:opacity-60">
        <Icon name="delete" tone="destructive" size={20} />
      </Pressable>
    </View>
  );
}

/** Saved shake-to-report records: thumbnail, note, date, screen path; delete one or all. */
export function FeedbackRecordsScreen() {
  const { t } = useTranslation('feedback');
  const records = useFeedback((s) => s.records);

  useEffect(() => {
    useFeedback.getState().refresh();
  }, []);

  const removeAll = () =>
    Alert.alert(t('delete_all_title'), t('delete_all_body'), [
      { text: t('cancel'), style: 'cancel' },
      { text: t('delete_all'), style: 'destructive', onPress: () => useFeedback.getState().removeAll() },
    ]);

  return (
    <>
      <Stack.Screen options={{ headerShown: true, title: t('records') }} />
      <Screen>
        {!records?.length ? (
          <EmptyState icon="shake" title={t('empty_title')} body={t('empty_body')} />
        ) : (
          <>
            <Text variant="footnote" tone="muted" className="px-1">
              {t('local_only')}
            </Text>
            <List>
              {records.map((r, i) => (
                <Row key={r.id} r={r} last={i === records.length - 1} />
              ))}
            </List>
            <ShareZipButton />
            <Button title={t('delete_all')} icon="delete" variant="ghost" onPress={removeAll} className="self-center" />
          </>
        )}
      </Screen>
    </>
  );
}
