import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { Chips } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  type AIActionCard,
  type ActionPreview,
  editsToSend,
  formatCountdown,
  initialEdits,
  missingRequired,
  secondsUntil,
} from './chat-state';
import { actionLabel, fieldLabel, warningLabel } from './labels';

/** Seconds left until `iso`, ticking every second while `active`. */
export function useSecondsLeft(iso: string, active = true) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [active]);
  return secondsUntil(iso, now);
}

export function cardTone(status: AIActionCard['status']): StatusTone {
  if (status === 'confirmed') return 'done';
  if (status === 'failed' || status === 'expired') return 'problem';
  if (status === 'executing' || status === 'pending') return 'progress';
  return 'neutral';
}

/** Preview rows (summary, fields, warnings) of a proposed action. */
export function ActionPreviewRows({ preview }: { preview: ActionPreview }) {
  const { t } = useTranslation('assistant');
  return (
    <>
      {preview.summary ? <Text variant="callout">{preview.summary}</Text> : null}
      {preview.fields?.length ? (
        <View className="gap-1">
          {preview.fields.map((f) => (
            <View key={f.key} className="flex-row gap-3">
              <Text variant="footnote" tone="muted" className="w-32">
                {fieldLabel(t, f.key)}
              </Text>
              <Text variant="footnote" className="flex-1" selectable>
                {f.value}
              </Text>
            </View>
          ))}
        </View>
      ) : null}
      {(preview.warnings ?? []).map((w) => (
        <Text key={w} variant="footnote" tone="danger">
          {warningLabel(t, w) ?? w}
        </Text>
      ))}
    </>
  );
}

/**
 * Confirmation card of a proposed write (TEC-387): preview rows, the
 * editable fields, warnings and a countdown to `expires_at`. Nothing runs
 * until the user confirms; confirm sends the edited values.
 */
export function ActionCard({
  card,
  busy,
  canConfirm,
  onConfirm,
  onCancel,
}: {
  card: AIActionCard;
  busy: boolean;
  canConfirm: boolean;
  onConfirm: (edits: Record<string, unknown> | undefined) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('assistant');
  const preview = (card.preview ?? {}) as ActionPreview;
  const pending = card.status === 'pending';
  const seconds = useSecondsLeft(card.expires_at, pending);
  const expired = card.status === 'expired' || (pending && seconds === 0);
  const editable = useMemo(() => preview.edit ?? [], [preview.edit]);
  const [values, setValues] = useState(() => initialEdits(preview));
  const set = (key: string, v: string) => setValues((s) => ({ ...s, [key]: v }));

  return (
    <View className="gap-3 rounded-lg border border-border bg-card p-4">
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="headline" className="flex-1">
          {actionLabel(t, preview.action ?? card.tool_name, preview.summary)}
        </Text>
        {pending && !expired ? (
          <Text variant="footnote" tone="muted" className="tabular-nums">
            {t('card.expires_in', { time: formatCountdown(seconds) })}
          </Text>
        ) : (
          <StatusPill label={t(`card.status_${expired ? 'expired' : card.status}`)} tone={cardTone(expired ? 'expired' : card.status)} />
        )}
      </View>
      <ActionPreviewRows preview={preview} />
      {pending && !expired && editable.length ? (
        <View className="gap-3">
          <Text variant="footnote" tone="muted">
            {t('card.edit_hint')}
          </Text>
          {editable.map((f) =>
            f.type === 'select' && f.options?.length ? (
              <View key={f.key} className="gap-1.5">
                <Text variant="footnote" tone="muted" className="font-medium">
                  {fieldLabel(t, f.key)}
                </Text>
                <Chips
                  value={values[f.key] ?? ''}
                  onChange={(v) => set(f.key, v)}
                  options={f.options.map((o) => ({ value: o, label: f.option_labels?.[o] ?? o }))}
                />
              </View>
            ) : (
              <Input
                key={f.key}
                label={fieldLabel(t, f.key)}
                placeholder={
                  f.type === 'date'
                    ? t('card.placeholder_date')
                    : f.type === 'time'
                      ? t('card.placeholder_time')
                      : t('card.placeholder_value')
                }
                value={values[f.key] ?? ''}
                onChangeText={(v) => set(f.key, v)}
                keyboardType={f.type === 'number' ? 'number-pad' : 'default'}
                multiline={f.type === 'textarea'}
                className={f.type === 'textarea' ? 'min-h-20 py-3' : undefined}
              />
            ),
          )}
        </View>
      ) : null}
      {expired ? (
        <Text variant="footnote" tone="danger">
          {t('card.expired')}
        </Text>
      ) : null}
      {pending && !expired ? (
        canConfirm ? (
          <View className="flex-row gap-2">
            <Button title={t('card.cancel')} variant="outline" disabled={busy} onPress={onCancel} className="flex-1" />
            <Button
              title={t('card.confirm')}
              disabled={busy || missingRequired(preview, values)}
              onPress={() => onConfirm(editsToSend(preview, values))}
              className="flex-1"
            />
          </View>
        ) : (
          <Text variant="footnote" tone="muted">
            {t('card.confirm_permission')}
          </Text>
        )
      ) : null}
    </View>
  );
}
