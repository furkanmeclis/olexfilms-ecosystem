import { Image } from 'expo-image';
import { type ComponentRef, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Dimensions,
  KeyboardAvoidingView,
  Modal,
  PanResponder,
  Platform,
  Pressable,
  ScrollView,
  View,
  type LayoutChangeEvent,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import Svg, { Path } from 'react-native-svg';

import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { loadViewShot } from './native';

/** Pen colors drawn on the screenshot; fixed (not themed) so they read the same in the ZIP. */
const COLORS = ['#e5383b', '#ffb703', '#2a9d8f', '#3a86ff', '#111111', '#ffffff'] as const;
const STROKE = 5;

type Stroke = { color: string; points: [number, number][] };

function pathOf(points: [number, number][]) {
  if (!points.length) return '';
  const [first, ...rest] = points;
  // A tap is a dot: a zero-length line with round caps.
  const tail = rest.length ? rest : [first];
  return `M${first[0]} ${first[1]} ${tail.map(([x, y]) => `L${x} ${y}`).join(' ')}`;
}

/** Largest box with the screenshot's aspect ratio inside the free area. */
function fit(area: { width: number; height: number }) {
  const screen = Dimensions.get('window');
  const ratio = screen.width / screen.height;
  const width = Math.min(area.width, area.height * ratio);
  return { width, height: width / ratio };
}

type Props = {
  imageUri: string;
  onCancel: () => void;
  /** `annotatedUri` is null when nothing was drawn. */
  onSave: (note: string, annotatedUri: string | null) => Promise<void>;
};

/**
 * Full-screen editor over the screenshot taken when the phone was shaken:
 * finger drawing (a few colors, undo, clear), a note, Save and Cancel.
 */
export function FeedbackEditor({ imageUri, onCancel, onSave }: Props) {
  const { t } = useTranslation('feedback');
  const insets = useSafeAreaInsets();
  const canvas = useRef<ComponentRef<typeof View>>(null);
  const [area, setArea] = useState<{ width: number; height: number } | null>(null);
  const [color, setColor] = useState<string>(COLORS[0]);
  const [strokes, setStrokes] = useState<Stroke[]>([]);
  const [note, setNote] = useState('');
  const [saving, setSaving] = useState(false);

  // A touch starts a new stroke; moves extend the last one (pure updaters, StrictMode-safe).
  const responder = useMemo(() => {
    const point = (x: number, y: number): [number, number] => [Math.round(x * 10) / 10, Math.round(y * 10) / 10];
    return PanResponder.create({
      onStartShouldSetPanResponder: () => true,
      onMoveShouldSetPanResponder: () => true,
      // Keep the stroke when a parent asks for the touch.
      onPanResponderTerminationRequest: () => false,
      onPanResponderGrant: (e) => {
        const p = point(e.nativeEvent.locationX, e.nativeEvent.locationY);
        setStrokes((s) => [...s, { color, points: [p] }]);
      },
      onPanResponderMove: (e) => {
        const p = point(e.nativeEvent.locationX, e.nativeEvent.locationY);
        setStrokes((s) => {
          const last = s[s.length - 1];
          return last ? [...s.slice(0, -1), { ...last, points: [...last.points, p] }] : s;
        });
      },
    });
  }, [color]);

  const dirty = strokes.length > 0 || note.trim().length > 0;

  const cancel = () => {
    if (!dirty) return onCancel();
    Alert.alert(t('discard_title'), t('discard_body'), [
      { text: t('keep_editing'), style: 'cancel' },
      { text: t('discard'), style: 'destructive', onPress: onCancel },
    ]);
  };

  const save = async () => {
    setSaving(true);
    try {
      let annotated: string | null = null;
      if (strokes.length) {
        const shot = loadViewShot();
        if (shot && canvas.current) {
          annotated = await shot.captureRef(canvas, { format: 'png', quality: 1, result: 'tmpfile' });
        }
      }
      await onSave(note.trim(), annotated);
    } catch (e) {
      Alert.alert(t('save_failed'), e instanceof Error ? e.message : String(e));
      setSaving(false);
    }
  };

  const onArea = (e: LayoutChangeEvent) => {
    const { width, height } = e.nativeEvent.layout;
    setArea({ width, height });
  };
  const box = area ? fit(area) : null;

  return (
    <Modal visible animationType="slide" presentationStyle="fullScreen" onRequestClose={cancel}>
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        className="flex-1 bg-background"
        style={{ paddingTop: insets.top, paddingBottom: insets.bottom }}>
        <View className="flex-row items-center gap-3 border-b border-border px-4 py-2">
          <Button title={t('cancel')} variant="ghost" onPress={cancel} disabled={saving} />
          <Text variant="headline" className="flex-1 text-center" numberOfLines={1}>
            {t('editor_title')}
          </Text>
          <Button title={t('save')} onPress={() => void save()} loading={saving} />
        </View>

        <View className="flex-1 items-center justify-center bg-muted p-3" onLayout={onArea}>
          {box ? (
            <View
              ref={canvas}
              collapsable={false}
              {...responder.panHandlers}
              className="overflow-hidden rounded-md border border-border"
              style={{ width: box.width, height: box.height }}>
              <Image source={{ uri: imageUri }} style={{ width: '100%', height: '100%' }} contentFit="fill" pointerEvents="none" />
              <Svg width={box.width} height={box.height} style={{ position: 'absolute', top: 0, left: 0 }} pointerEvents="none">
                {strokes.map((s, i) => (
                  <Path
                    key={i}
                    d={pathOf(s.points)}
                    stroke={s.color}
                    strokeWidth={STROKE}
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    fill="none"
                  />
                ))}
              </Svg>
            </View>
          ) : null}
        </View>

        <View className="gap-3 border-t border-border px-4 pt-3">
          <View className="flex-row items-center gap-2">
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerClassName="gap-2" className="flex-1">
              {COLORS.map((c) => (
                <Pressable
                  key={c}
                  accessibilityRole="button"
                  accessibilityLabel={t('pen_color')}
                  accessibilityState={{ selected: c === color }}
                  onPress={() => setColor(c)}
                  className={cn(
                    'size-9 items-center justify-center rounded-full border-2',
                    c === color ? 'border-foreground' : 'border-transparent',
                  )}>
                  <View className="size-7 rounded-full border border-border" style={{ backgroundColor: c }} />
                </Pressable>
              ))}
            </ScrollView>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('undo')}
              disabled={!strokes.length}
              onPress={() => setStrokes((s) => s.slice(0, -1))}
              hitSlop={8}
              className={cn('size-10 items-center justify-center rounded-md active:bg-muted', !strokes.length && 'opacity-40')}>
              <Icon name="undo" tone="primary" size={20} />
            </Pressable>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('clear')}
              disabled={!strokes.length}
              onPress={() => setStrokes([])}
              hitSlop={8}
              className={cn('size-10 items-center justify-center rounded-md active:bg-muted', !strokes.length && 'opacity-40')}>
              <Icon name="delete" tone="destructive" size={20} />
            </Pressable>
          </View>
          <Input
            label={t('note')}
            placeholder={t('note_placeholder')}
            value={note}
            onChangeText={setNote}
            multiline
            maxLength={2000}
            className="max-h-28 min-h-[52px] py-3"
            textAlignVertical="top"
          />
          <View className="h-2" />
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}
