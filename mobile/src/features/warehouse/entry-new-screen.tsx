import { useQueryClient } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { activeMembership } from '@/core/spaces';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { entryApi, wk } from './api';
import { useCurrentWarehouse } from './context';
import { alertError, replace } from './format';
import { Chips } from './parts';
import { WarehouseField } from './warehouse-picker';

/**
 * Opens a stock entry: printed labels (with_existing) or, at the center, new
 * barcodes reserved on the entry (generate_new).
 */
export function EntryNewScreen() {
  const { t } = useTranslation('warehouse');
  const qc = useQueryClient();
  const { warehouse } = useCurrentWarehouse();
  const center = useSession((s) => activeMembership(s.me)?.type === 'center');
  const [mode, setMode] = useState<'with_existing' | 'generate_new'>('with_existing');
  const [wh, setWh] = useState<string | undefined>(undefined);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);
  const whId = wh ?? warehouse?.uuid;

  const submit = async () => {
    setTried(true);
    if (!whId) return;
    setBusy(true);
    try {
      const x = await entryApi.create({ warehouse_uuid: whId, mode: center ? mode : 'with_existing', note: note.trim() || null });
      qc.setQueryData(wk.entry(x.uuid), x);
      void qc.invalidateQueries({ queryKey: ['warehouse', 'entries'] });
      void qc.invalidateQueries({ queryKey: wk.summary() });
      replace(`/depo/jobs/entry/${x.uuid}`);
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: t('entries.new_title'), headerLargeTitle: false }} />
      <Screen>
        {center ? (
          <>
            <Chips
              value={mode}
              onChange={setMode}
              options={[
                { value: 'with_existing', label: t('entry_mode.with_existing') },
                { value: 'generate_new', label: t('entry_mode.generate_new') },
              ]}
            />
            <View className="flex-row items-center gap-2">
              <Icon name="barcode" tone="muted-foreground" size={16} />
              <Text variant="callout" tone="muted" className="flex-1">
                {t('entries.mode_hint_center')}
              </Text>
            </View>
          </>
        ) : (
          <View className="flex-row items-center gap-2">
            <Icon name="scan" tone="muted-foreground" size={16} />
            <Text variant="callout" tone="muted" className="flex-1">
              {t('entries.mode_hint_distributor')}
            </Text>
          </View>
        )}
        <WarehouseField
          label={t('fields.warehouse')}
          value={whId}
          onChange={(w) => setWh(w.uuid)}
          error={tried && !whId ? t('validation.warehouse') : undefined}
        />
        <Input label={t('fields.note')} placeholder={t('placeholder_note')} value={note} onChangeText={setNote} multiline maxLength={500} className="min-h-20 py-3" />
        <Button size="lg" icon={<Icon name="packageIn" tone="primary-foreground" size={18} />} title={t('entries.create')} loading={busy} onPress={() => void submit()} />
      </Screen>
    </>
  );
}
