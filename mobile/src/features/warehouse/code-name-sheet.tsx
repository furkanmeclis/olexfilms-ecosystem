import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardAvoidingView, Modal, Platform, ScrollView, View } from 'react-native';

import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { whError } from './format';
import { Chips } from './parts';
import { CODE_RE, type LocationType, normalizeCode } from './setup-api';

export type CodeNameValues = { code: string; name: string; type?: LocationType };

/**
 * Small form sheet for a room or location: code (A-Z, 0-9, _), optional name
 * and, for a new location, its type. Server errors stay in the sheet.
 */
export function CodeNameSheet({
  title,
  initial,
  types,
  onClose,
  onSubmit,
}: {
  title: string;
  initial?: Partial<CodeNameValues>;
  types?: LocationType[];
  onClose: () => void;
  onSubmit: (v: CodeNameValues) => Promise<unknown>;
}) {
  const { t } = useTranslation('warehouse');
  const [code, setCode] = useState(initial?.code ?? '');
  const [name, setName] = useState(initial?.name ?? '');
  const [type, setType] = useState<LocationType | undefined>(initial?.type ?? types?.[0]);
  const [tried, setTried] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const norm = normalizeCode(code);
  const codeError = tried && !CODE_RE.test(norm) ? t('validation.code') : undefined;

  const submit = async () => {
    setTried(true);
    if (!CODE_RE.test(norm)) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit({ code: norm, name: name.trim(), type });
      onClose();
    } catch (e) {
      setError(whError(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title" className="flex-1" numberOfLines={1}>
            {title}
          </Text>
          <Button title={t('common:cancel')} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="gap-4 px-4 pb-10" keyboardShouldPersistTaps="handled">
          {types && types.length > 1 ? (
            <View className="gap-2">
              <Text variant="footnote" tone="muted" className="font-medium">
                {t('fields.type')}
              </Text>
              <Chips value={type ?? types[0]} onChange={setType} options={types.map((x) => ({ value: x, label: t(`type.${x}`) }))} />
            </View>
          ) : null}
          <Input
            label={t('fields.code')}
            value={code}
            onChangeText={setCode}
            autoCapitalize="characters"
            autoCorrect={false}
            autoFocus
            hint={t('form.code_hint')}
            error={codeError}
          />
          <Input label={t('fields.name')} value={name} onChangeText={setName} hint={t('form.name_hint')} />
          {error ? (
            <Text variant="footnote" tone="danger">
              {error}
            </Text>
          ) : null}
          <Button size="lg" title={t('form.save')} loading={busy} onPress={() => void submit()} />
        </ScrollView>
      </KeyboardAvoidingView>
    </Modal>
  );
}
