import { useQueryClient } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { transferApi, wk } from './api';
import { useCurrentWarehouse } from './context';
import { alertError, replace } from './format';
import { ListSkeleton } from './parts';
import { WarehouseField } from './warehouse-picker';

/** Opens a draft transfer between two warehouses of the organization. */
export function TransferNewScreen() {
  const { t } = useTranslation('warehouse');
  const qc = useQueryClient();
  const { warehouse, warehouses, isLoading } = useCurrentWarehouse();
  const [from, setFrom] = useState<string | undefined>(undefined);
  const [to, setTo] = useState<string | undefined>(undefined);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);

  const fromId = from ?? warehouse?.uuid;
  const toId = to ?? warehouses.find((w) => w.uuid !== fromId)?.uuid;
  const same = !!fromId && fromId === toId;

  const submit = async () => {
    setTried(true);
    if (!fromId || !toId || same || note.length > 500) return;
    setBusy(true);
    try {
      const x = await transferApi.create({ from_warehouse_uuid: fromId, to_warehouse_uuid: toId, note: note.trim() || null });
      qc.setQueryData(wk.transfer(x.uuid), x);
      void qc.invalidateQueries({ queryKey: ['warehouse', 'transfers'] });
      void qc.invalidateQueries({ queryKey: wk.summary() });
      replace(`/depo/jobs/transfer/${x.uuid}`);
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: t('transfers.new_title'), headerLargeTitle: false }} />
      <Screen>
        {isLoading ? (
          <ListSkeleton rows={3} />
        ) : warehouses.length < 2 ? (
          <EmptyState icon="warehouse" title={t('transfers.need_two_warehouses')} />
        ) : (
          <>
            <Text variant="callout" tone="muted">
              {t('transfers.empty_description')}
            </Text>
            <WarehouseField label={t('fields.from_warehouse')} value={fromId} onChange={(w) => setFrom(w.uuid)} />
            <WarehouseField
              label={t('fields.to_warehouse')}
              value={toId}
              exclude={fromId}
              onChange={(w) => setTo(w.uuid)}
              error={tried && same ? t('validation.same_warehouse') : undefined}
            />
            <Input
              label={t('fields.note')}
              value={note}
              onChangeText={setNote}
              multiline
              maxLength={500}
              className="min-h-24 py-3"
              error={tried && note.length > 500 ? t('validation.too_long', { max: 500 }) : undefined}
            />
            <Button size="lg" title={t('transfers.create')} loading={busy} onPress={() => void submit()} />
          </>
        )}
      </Screen>
    </>
  );
}
