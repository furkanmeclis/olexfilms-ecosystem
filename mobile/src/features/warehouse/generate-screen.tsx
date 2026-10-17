import { useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, View } from 'react-native';

import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { alertError } from './format';
import { Card, Chips, IconButton } from './parts';
import { allowedChildTypes, CODE_RE, type GenerateLevel, type LocationType, normalizeCode, setupApi } from './setup-api';

/** Same limits as the web wizard and the backend. */
const GENERATE_MAX = 2000;
const RANGE_MAX = 9999;
const PAD_MAX = 4;

type Level = { type: LocationType; mode: 'range' | 'list'; prefix: string; from: string; to: string; pad: string; codes: string };

function newLevel(type: LocationType): Level {
  return type === 'aisle'
    ? { type, mode: 'list', prefix: '', from: '1', to: '2', pad: '0', codes: 'A, B' }
    : { type, mode: 'range', prefix: '', from: '1', to: type === 'shelf' ? '5' : '4', pad: '2', codes: '' };
}

function defaultLevels(parent: LocationType | null): Level[] {
  const out: Level[] = [];
  let prev = parent;
  for (;;) {
    const next = allowedChildTypes(prev)[0];
    if (!next) break;
    out.push(newLevel(next));
    prev = next;
  }
  return out;
}

const splitCodes = (raw: string) =>
  raw
    .split(/[\s,;]+/)
    .map((s) => s.trim())
    .filter(Boolean);

/** Expands a level like the backend: prefix + zero-padded numbers, or the list. Returns codes or an error key. */
function levelCodes(lv: Level): { codes: string[] } | { error: string; code?: string } {
  let raw: string[];
  if (lv.mode === 'list') {
    raw = splitCodes(lv.codes);
    if (!raw.length) return { error: 'bulk.error.codes_required' };
  } else {
    const int = /^\d+$/;
    if (!int.test(lv.from.trim()) || !int.test(lv.to.trim())) return { error: 'bulk.error.range' };
    const from = Number(lv.from);
    const to = Number(lv.to);
    if (from > to || to > RANGE_MAX || to - from + 1 > GENERATE_MAX) return { error: 'bulk.error.range' };
    const padRaw = lv.pad.trim() || '0';
    if (!int.test(padRaw) || Number(padRaw) > PAD_MAX) return { error: 'bulk.error.pad' };
    raw = [];
    for (let n = from; n <= to; n++) raw.push(`${lv.prefix.trim()}${String(n).padStart(Number(padRaw), '0')}`);
  }
  const seen = new Set<string>();
  const codes: string[] = [];
  for (const r of raw) {
    const c = normalizeCode(r);
    if (!CODE_RE.test(c)) return { error: 'bulk.error.code_invalid', code: c };
    if (seen.has(c)) return { error: 'bulk.error.duplicate', code: c };
    seen.add(c);
    codes.push(c);
  }
  return { codes };
}

function toApi(lv: Level): GenerateLevel {
  if (lv.mode === 'list') return { type: lv.type, codes: splitCodes(lv.codes).map(normalizeCode) };
  return { type: lv.type, from: Number(lv.from), to: Number(lv.to), pad: Number(lv.pad.trim() || '0'), prefix: normalizeCode(lv.prefix) };
}

/**
 * Bulk create a grid of aisles, shelves and bins under a room or a location
 * (POST /v1/warehouse/locations/generate). Existing codes are kept and reused.
 */
export function GenerateScreen() {
  const { t } = useTranslation('warehouse');
  const qc = useQueryClient();
  const p = useLocalSearchParams<{ room: string; parent?: string; ptype?: LocationType; pcode?: string }>();
  const parentType = p.ptype ?? null;
  const [levels, setLevels] = useState<Level[]>(() => defaultLevels(parentType));
  const [busy, setBusy] = useState(false);

  const results = levels.map(levelCodes);
  const counts = results.map((r) => ('codes' in r ? r.codes.length : 0));
  const total = counts.reduce((sum, _, i) => sum + counts.slice(0, i + 1).reduce((a, b) => a * b, 1), 0);
  const valid = levels.length > 0 && results.every((r) => 'codes' in r);
  const sample = (() => {
    if (!valid) return [];
    const lists = results.map((r) => ('codes' in r ? r.codes : []));
    const out: string[] = [];
    const walk = (i: number, prefix: string) => {
      if (out.length >= 6) return;
      if (i === lists.length) return void out.push(prefix);
      for (const c of lists[i]) {
        walk(i + 1, prefix ? `${prefix}-${c}` : c);
        if (out.length >= 6) return;
      }
    };
    walk(0, p.pcode ?? '');
    return out;
  })();

  const update = (i: number, patch: Partial<Level>) => setLevels((ls) => ls.map((l, k) => (k === i ? { ...l, ...patch } : l)));

  const lastType = levels.length ? levels[levels.length - 1].type : parentType;
  const nextType = allowedChildTypes(lastType)[0];

  const submit = () => {
    if (!valid || total > GENERATE_MAX) return;
    const go = async () => {
      setBusy(true);
      try {
        const r = await setupApi.generate({ room_uuid: p.room, parent_uuid: p.parent ?? null, levels: levels.map(toApi) });
        void qc.invalidateQueries({ queryKey: ['warehouse', 'setup'] });
        Alert.alert(t('bulk.result.created', { count: r.created }), t('bulk.result.existing', { count: r.existing }));
        router.back();
      } catch (e) {
        alertError(e);
      } finally {
        setBusy(false);
      }
    };
    if (total >= 500) {
      Alert.alert(t('bulk.preview.large', { count: total }), undefined, [
        { text: t('common:cancel'), style: 'cancel' },
        { text: t('bulk.create', { count: total }), onPress: () => void go() },
      ]);
    } else void go();
  };

  return (
    <>
      <Stack.Screen options={{ title: p.pcode ? t('bulk.open_under', { code: p.pcode }) : t('bulk.title'), headerLargeTitle: false }} />
      <Screen>
        <Text variant="callout" tone="muted">
          {t('bulk.description')}
        </Text>
        {levels.map((lv, i) => {
          const r = results[i];
          return (
            <Card key={i} className="gap-3">
              <View className="flex-row items-center justify-between">
                <Text variant="headline">
                  {t('bulk.level_title', { n: i + 1 })} · {t(`type.${lv.type}`)}
                </Text>
                {i === levels.length - 1 ? (
                  <IconButton icon="delete" tone="destructive" label={t('bulk.remove_level')} onPress={() => setLevels((ls) => ls.slice(0, -1))} />
                ) : null}
              </View>
              {allowedChildTypes(i === 0 ? parentType : levels[i - 1].type).length > 1 ? (
                <Chips
                  value={lv.type}
                  onChange={(type) => setLevels((ls) => [...ls.slice(0, i), { ...ls[i], type }])}
                  options={allowedChildTypes(i === 0 ? parentType : levels[i - 1].type).map((x) => ({ value: x, label: t(`type.${x}`) }))}
                />
              ) : null}
              <Chips
                value={lv.mode}
                onChange={(mode) => update(i, { mode })}
                options={[
                  { value: 'range', label: t('bulk.mode.range') },
                  { value: 'list', label: t('bulk.mode.list') },
                ]}
              />
              {lv.mode === 'list' ? (
                <Input label={t('bulk.codes')} hint={t('bulk.codes_hint')} value={lv.codes} onChangeText={(codes) => update(i, { codes })} autoCapitalize="characters" autoCorrect={false} />
              ) : (
                <>
                  <Input label={t('bulk.prefix')} hint={t('bulk.prefix_hint')} value={lv.prefix} onChangeText={(prefix) => update(i, { prefix })} autoCapitalize="characters" autoCorrect={false} />
                  <View className="flex-row gap-2">
                    <View className="flex-1">
                      <Input label={t('bulk.from')} value={lv.from} onChangeText={(from) => update(i, { from })} keyboardType="number-pad" />
                    </View>
                    <View className="flex-1">
                      <Input label={t('bulk.to')} value={lv.to} onChangeText={(to) => update(i, { to })} keyboardType="number-pad" />
                    </View>
                    <View className="flex-1">
                      <Input label={t('bulk.pad')} value={lv.pad} onChangeText={(pad) => update(i, { pad })} keyboardType="number-pad" />
                    </View>
                  </View>
                </>
              )}
              {'error' in r ? (
                <Text variant="footnote" tone="danger">
                  {t(r.error, { code: r.code ?? '' })}
                </Text>
              ) : (
                <Text variant="footnote" tone="muted" numberOfLines={1}>
                  {r.codes.slice(0, 8).join(', ')}
                  {r.codes.length > 8 ? ' …' : ''}
                </Text>
              )}
            </Card>
          );
        })}
        {nextType ? (
          <Button variant="outline" title={t('bulk.add_level', { type: t(`type.${nextType}`) })} onPress={() => setLevels((ls) => [...ls, newLevel(nextType)])} />
        ) : null}

        <Card>
          <Field2 label={t('bulk.preview.total')} value={valid ? String(total) : '–'} />
          {sample.length ? <Field2 label={t('bulk.preview.sample')} value={sample.join(', ')} /> : null}
          <Text variant="footnote" tone="muted">
            {t('bulk.preview.existing_hint')}
          </Text>
          {total > GENERATE_MAX ? (
            <Text variant="footnote" tone="danger">
              {t('bulk.preview.too_many', { count: total, max: GENERATE_MAX })}
            </Text>
          ) : null}
        </Card>
        <Button size="lg" title={t('bulk.create', { count: valid ? total : 0 })} disabled={!valid || total === 0 || total > GENERATE_MAX} loading={busy} onPress={submit} />
      </Screen>
    </>
  );
}

function Field2({ label, value }: { label: string; value: string }) {
  return (
    <View className="gap-0.5 py-1">
      <Text variant="footnote" tone="muted">
        {label}
      </Text>
      <Text variant="callout" className="font-mono">
        {value}
      </Text>
    </View>
  );
}
