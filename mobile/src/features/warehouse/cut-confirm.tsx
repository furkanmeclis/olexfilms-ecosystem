import { useEffect, useRef, useState, type ComponentRef } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, ScrollView, TextInput, View } from 'react-native';

import type { components } from '@/core/api/schema';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { formatNumber, whError } from './format';
import { cutWarnings, pieceCredits, pieceMeters } from './order-assign';

export type CutPreview = components['schemas']['OrderCutPreview'];
export type PartialInput = { meters: string; note: string; credits: string };

/**
 * TEC-1241: the full-screen confirmation of a scanned roll or closed box on
 * a meter line, shown over the scanner. It names what happens ("closed box:
 * 3 m are cut, 12 m stay as an open roll"), warns about a remnant and the
 * credits the piece carries. A hardware reader's Enter (or the big button)
 * confirms; scanning is paused meanwhile. The partial form (K11) asks for
 * the piece length, a note and the credits.
 */
export function CutConfirm({
  barcode,
  productName,
  partialStart,
  load,
  onConfirm,
  onCancel,
}: {
  barcode: string;
  productName: string;
  partialStart: boolean;
  /** Reads the preview for the typed piece meters ('' = the default cut). */
  load: (meters: string) => Promise<CutPreview>;
  onConfirm: (preview: CutPreview, partial: PartialInput | null) => Promise<void>;
  onCancel: () => void;
}) {
  const { t } = useTranslation('warehouse');
  const [preview, setPreview] = useState<CutPreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [partial, setPartial] = useState(partialStart);
  const [meters, setMeters] = useState('');
  const [note, setNote] = useState('');
  const [credits, setCredits] = useState('');
  const [busy, setBusy] = useState(false);
  const enter = useRef<ComponentRef<typeof TextInput>>(null);
  const typed = partial ? pieceMeters(meters) : '';

  useEffect(() => {
    let live = true;
    load(typed)
      .then((p) => {
        if (!live) return;
        setPreview(p);
        setError(null);
      })
      .catch((e: unknown) => {
        if (!live) return;
        setPreview(null);
        setError(whError(e));
      });
    return () => {
      live = false;
    };
    // load is recreated by the caller on each render; the cut depends on the typed meters only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [typed]);

  const needsNote = !!preview?.partial;
  const c = pieceCredits(credits);
  const ready =
    !!preview &&
    !busy &&
    (!needsNote || (partial && note.trim() !== '')) &&
    !(c != null && Number.isNaN(c)) &&
    (!partial || meters.trim() === '' || typed !== '');

  const confirm = async () => {
    if (!ready || !preview) return;
    setBusy(true);
    try {
      await onConfirm(preview, partial ? { meters, note, credits } : null);
    } catch (e) {
      setError(whError(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <View className="absolute inset-0 justify-end bg-black/70">
      <ScrollView
        className="max-h-[90%] rounded-t-2xl bg-card"
        contentContainerClassName="gap-3 p-4 pb-8"
        keyboardShouldPersistTaps="handled">
        <View className="flex-row items-center gap-2">
          <Icon name={preview?.closed_box ? 'package' : 'tools'} tone="primary" size={22} />
          <Text variant="title" className="flex-1" numberOfLines={2}>
            {preview?.closed_box ? t('orders.cut.title_box') : t('orders.cut.title')}
          </Text>
        </View>
        <Text variant="footnote" tone="muted" numberOfLines={2}>
          {productName} · <Text variant="footnote" tone="muted" className="font-mono">{barcode}</Text>
          {preview?.location_code ? ` · ${preview.location_code}` : ''}
        </Text>

        {!preview && !error ? <ActivityIndicator /> : null}
        {error ? (
          <Text variant="callout" tone="danger">
            {error}
          </Text>
        ) : null}

        {preview ? (
          <>
            <View className="flex-row gap-2">
              {[
                [preview.closed_box ? t('orders.cut.box') : t('orders.cut.roll'), preview.source_meters, false],
                [t('orders.cut.cut'), preview.cut_meters, true],
                [t('orders.cut.rest'), preview.remaining_meters, false],
              ].map(([label, value, strong]) => (
                <View key={String(label)} className="flex-1 items-center rounded-lg border border-border py-2">
                  <Text variant="footnote" tone="muted">
                    {String(label)}
                  </Text>
                  <Text variant="title" tone={strong ? 'primary' : 'default'}>
                    {formatNumber(String(value), 2)} m
                  </Text>
                </View>
              ))}
            </View>
            {cutWarnings(preview).map((w) => (
              <View key={w.key} className="flex-row items-start gap-2 rounded-lg bg-secondary p-3">
                <Icon name="alert" tone="destructive" size={18} />
                <Text variant="callout" className="flex-1">
                  {t(w.key, w.values)}
                </Text>
              </View>
            ))}
            <Text variant="callout">
              {t('orders.cut.credits', { credits: preview.credits, line: preview.line_credits })}
              {preview.plotter_cut ? ` · ${t('orders.cut.plotter_mark')}` : ''}
            </Text>
            {needsNote && !partial ? (
              <Button variant="outline" title={t('orders.cut.partial_button')} onPress={() => setPartial(true)} />
            ) : null}
          </>
        ) : null}

        {partial ? (
          <View className="gap-2 rounded-lg border border-border p-3">
            <Text variant="headline">{t('orders.cut.partial_title')}</Text>
            <Text variant="footnote" tone="muted">
              {t('orders.cut.partial_hint')}
            </Text>
            <View className="flex-row gap-2">
              <View className="flex-1 gap-1">
                <Text variant="footnote" tone="muted">
                  {t('orders.cut.piece_meters')}
                </Text>
                <TextInput
                  value={meters}
                  onChangeText={setMeters}
                  keyboardType="decimal-pad"
                  placeholder={preview?.cut_meters}
                  placeholderTextColorClassName="accent-muted-foreground"
                  className="min-h-11 rounded-md border border-border px-2 text-center text-[17px] text-foreground"
                />
              </View>
              <View className="flex-1 gap-1">
                <Text variant="footnote" tone="muted">
                  {t('orders.cut.piece_credits')}
                </Text>
                <TextInput
                  value={credits}
                  onChangeText={setCredits}
                  keyboardType="number-pad"
                  placeholder={preview ? String(preview.credits) : ''}
                  placeholderTextColorClassName="accent-muted-foreground"
                  className="min-h-11 rounded-md border border-border px-2 text-center text-[17px] text-foreground"
                />
              </View>
            </View>
            <Text variant="footnote" tone="muted">
              {t('orders.cut.note')}
            </Text>
            <TextInput
              value={note}
              onChangeText={setNote}
              maxLength={500}
              multiline
              className="min-h-16 rounded-md border border-border px-2 py-2 text-[17px] text-foreground"
            />
          </View>
        ) : null}

        <Button
          size="lg"
          title={preview?.closed_box ? t('orders.cut.confirm_box') : t('orders.cut.confirm')}
          icon="check"
          loading={busy}
          disabled={!ready}
          onPress={() => void confirm()}
        />
        <Button variant="ghost" title={t('common:cancel')} onPress={onCancel} />
        {/* A hardware reader's Enter confirms (keyboard wedge); no soft keyboard. */}
        {!partial ? (
          <TextInput
            ref={enter}
            autoFocus
            showSoftInputOnFocus={false}
            caretHidden
            blurOnSubmit={false}
            value=""
            onSubmitEditing={() => void confirm()}
            accessibilityElementsHidden
            importantForAccessibility="no-hide-descendants"
            style={{ position: 'absolute', width: 1, height: 1, opacity: 0, left: 0, top: 0 }}
          />
        ) : null}
      </ScrollView>
    </View>
  );
}
