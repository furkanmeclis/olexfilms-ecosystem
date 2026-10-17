import { useQueryClient } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { entryApi, wk } from './api';
import { useCurrentWarehouse } from './context';
import { alertError, replace } from './format';
import { WarehouseField } from './warehouse-picker';

/**
 * Opens a stock entry for printed labels (mode with_existing). Reserving new
 * barcodes (generate_new) needs label printing and stays on the web for now.
 */
export function EntryNewScreen() {
  const { t } = useTranslation('warehouse');
  const qc = useQueryClient();
  const { warehouse } = useCurrentWarehouse();
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
      const x = await entryApi.create({ warehouse_uuid: whId, mode: 'with_existing', note: note.trim() || null });
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
        <Text variant="callout" tone="muted">
          {t('entries.mode_hint_distributor')}
        </Text>
        <WarehouseField
          label={t('fields.warehouse')}
          value={whId}
          onChange={(w) => setWh(w.uuid)}
          error={tried && !whId ? t('validation.warehouse') : undefined}
        />
        <Input label={t('fields.note')} value={note} onChangeText={setNote} multiline maxLength={500} className="min-h-20 py-3" />
        <Button size="lg" title={t('entries.create')} loading={busy} onPress={() => void submit()} />
      </Screen>
    </>
  );
}
