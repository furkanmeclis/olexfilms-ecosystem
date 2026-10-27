import { type ComponentRef, useEffect, useRef, useState } from 'react';
import { Pressable, TextInput, View } from 'react-native';
import Animated, {
  useAnimatedStyle,
  useReducedMotion,
  useSharedValue,
  withRepeat,
  withSequence,
  withTiming,
  ZoomIn,
} from 'react-native-reanimated';

import { toLatinDigits } from '@/core/digits';
import { cn } from '@/ui/cn';
import { Text } from '@/ui/text';

type Props = {
  value: string;
  onChange: (code: string) => void;
  /** Called once when the last box fills (typing, paste or iOS autofill). */
  onComplete?: (code: string) => void;
  length?: number;
  /** Red boxes; a change of `errorKey` also shakes the row. */
  error?: boolean;
  errorKey?: number;
  disabled?: boolean;
  accessibilityLabel: string;
};

/**
 * One-time code in separate boxes. A single transparent TextInput over the row
 * takes the keyboard, paste and the iOS "From Messages" autofill, so the boxes
 * only draw the value.
 */
export function CodeInput({
  value,
  onChange,
  onComplete,
  length = 6,
  error,
  errorKey = 0,
  disabled,
  accessibilityLabel,
}: Props) {
  const input = useRef<ComponentRef<typeof TextInput>>(null);
  const [focused, setFocused] = useState(false);
  const reduceMotion = useReducedMotion();
  const shake = useSharedValue(0);
  const caret = useSharedValue(1);

  useEffect(() => {
    if (!errorKey || reduceMotion) return;
    shake.value = withSequence(
      withTiming(-10, { duration: 50 }),
      withTiming(10, { duration: 50 }),
      withTiming(-7, { duration: 50 }),
      withTiming(7, { duration: 50 }),
      withTiming(0, { duration: 50 }),
    );
  }, [errorKey, reduceMotion, shake]);

  useEffect(() => {
    caret.value = reduceMotion ? 1 : withRepeat(withSequence(withTiming(0, { duration: 500 }), withTiming(1, { duration: 500 })), -1);
  }, [caret, reduceMotion]);

  // A cleared code (wrong code, resend) puts the cursor back in the first box.
  useEffect(() => {
    if (!value) input.current?.focus();
  }, [value]);

  const rowStyle = useAnimatedStyle(() => ({ transform: [{ translateX: shake.value }] }));
  const caretStyle = useAnimatedStyle(() => ({ opacity: caret.value }));

  const change = (text: string) => {
    if (disabled) return;
    const digits = toLatinDigits(text).replace(/\D/g, '').slice(0, length);
    onChange(digits);
    if (digits.length === length) onComplete?.(digits);
  };

  const active = Math.min(value.length, length - 1);
  const half = length % 2 === 0 ? length / 2 : -1;

  return (
    <Pressable onPress={() => input.current?.focus()} accessible={false}>
      {/* Digits read left to right in every language. */}
      <Animated.View style={[{ direction: 'ltr' }, rowStyle]} className="flex-row items-center justify-center gap-2">
        {Array.from({ length }, (_, i) => {
          const digit = value[i];
          const current = focused && !disabled && i === active && value.length < length;
          return (
            <View key={i} className="flex-row items-center gap-2">
              {i === half ? <View className="h-0.5 w-2.5 rounded-full bg-border" /> : null}
              <View
                className={cn(
                  'h-14 w-11 items-center justify-center rounded-2xl border-2 bg-card',
                  error ? 'border-destructive' : current ? 'border-primary' : digit ? 'border-foreground/25' : 'border-border',
                )}
                style={{ borderCurve: 'continuous' }}>
                {digit ? (
                  <Animated.View key={digit + i} entering={reduceMotion ? undefined : ZoomIn.duration(140)}>
                    <Text className={cn('text-[26px] font-semibold tabular-nums', error && 'text-destructive')}>{digit}</Text>
                  </Animated.View>
                ) : current ? (
                  <Animated.View style={caretStyle} className="h-7 w-0.5 rounded-full bg-primary" />
                ) : null}
              </View>
            </View>
          );
        })}
      </Animated.View>
      <TextInput
        ref={input}
        value={value}
        onChangeText={change}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        // Stays editable while busy: a non-editable field drops the keyboard and it would jump back.
        maxLength={length}
        keyboardType="number-pad"
        textContentType="oneTimeCode"
        autoComplete="one-time-code"
        autoFocus
        caretHidden
        contextMenuHidden={false}
        accessibilityLabel={accessibilityLabel}
        // Nearly invisible but still hit-testable, so long-press paste works on the boxes.
        className="absolute inset-0 text-transparent"
        style={{ opacity: 0.02 }}
      />
    </Pressable>
  );
}
