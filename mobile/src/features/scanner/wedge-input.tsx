import { type ComponentRef, useEffect, useRef, useState } from 'react';
import { TextInput } from 'react-native';

import { splitWedgeInput, WEDGE_IDLE_MS, WEDGE_MIN_LENGTH } from './wedge';

/**
 * Hidden input that catches a keyboard-wedge reader (TEC-1230): it keeps
 * the focus without opening the on-screen keyboard, collects the
 * keystrokes and hands each code to `onCode` at Enter / Tab (or when the
 * typing stops, for readers without a terminator). `paused` gives the
 * focus back to the screen (manual entry, a dialog).
 */
export function WedgeInput({ onCode, paused }: { onCode: (code: string) => void; paused?: boolean }) {
  const ref = useRef<ComponentRef<typeof TextInput>>(null);
  const [value, setValue] = useState('');
  const buffer = useRef('');
  const idle = useRef<ReturnType<typeof setTimeout> | null>(null);
  const emit = useRef(onCode);
  useEffect(() => {
    emit.current = onCode;
  }, [onCode]);

  useEffect(() => {
    if (paused) {
      ref.current?.blur();
      return;
    }
    const id = setTimeout(() => ref.current?.focus(), 50);
    return () => clearTimeout(id);
  }, [paused]);

  useEffect(
    () => () => {
      if (idle.current) clearTimeout(idle.current);
    },
    [],
  );

  const flush = () => {
    const code = buffer.current.trim();
    buffer.current = '';
    setValue('');
    if (code.length >= WEDGE_MIN_LENGTH) emit.current(code);
  };

  return (
    <TextInput
      ref={ref}
      value={value}
      autoFocus={!paused}
      showSoftInputOnFocus={false}
      caretHidden
      contextMenuHidden
      autoCapitalize="none"
      autoCorrect={false}
      autoComplete="off"
      spellCheck={false}
      blurOnSubmit={false}
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      style={{
        position: 'absolute',
        width: 1,
        height: 1,
        opacity: 0,
        left: 0,
        top: 0,
      }}
      onChangeText={(text) => {
        const { codes, rest } = splitWedgeInput(text);
        buffer.current = rest;
        setValue(rest);
        for (const c of codes) emit.current(c);
        if (idle.current) clearTimeout(idle.current);
        if (rest) idle.current = setTimeout(flush, WEDGE_IDLE_MS);
      }}
      onSubmitEditing={() => {
        if (idle.current) clearTimeout(idle.current);
        flush();
      }}
      onBlur={() => {
        // A reader needs the focus back (a tap elsewhere, a toast).
        if (!paused) setTimeout(() => ref.current?.focus(), 150);
      }}
    />
  );
}
