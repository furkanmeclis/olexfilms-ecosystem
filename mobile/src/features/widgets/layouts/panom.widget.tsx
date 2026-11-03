// Panom widget'ı: kilit ekranı dalları tamam, ana ekran boyları iskelet (kuyruk satırları sonraki görevde).
// `'widget'` gövdesi metne çevrilip extension'da çalışır; yardımcılar bilinçli olarak `koruma.widget.tsx`'in kopyasıdır.
// `state !== 'ok'` iken `props` yalnız BaseProps'tur: bu dallar `card` dışında veri okumaz.
import type { CommonViewModifierProps } from '@expo/ui/swift-ui';
import { AccessoryWidgetBackground, HStack, Image, Spacer, Text, VStack, ZStack } from '@expo/ui/swift-ui';
import {
  background,
  clipShape,
  containerBackground,
  font,
  foregroundStyle,
  frame,
  lineLimit,
  minimumScaleFactor,
  monospacedDigit,
  multilineTextAlignment,
  padding,
  resizable,
  widgetURL,
} from '@expo/ui/swift-ui/modifiers';
import type { WidgetEnvironment } from 'expo-widgets';
import { createWidget } from 'expo-widgets';
import type { ReactNode } from 'react';

import type { BaseProps, PanomProps, WidgetCard } from '../types';

type Mods = NonNullable<CommonViewModifierProps['modifiers']>;
type Weight = 'regular' | 'semibold' | 'bold';

const PanomLayout = (props: PanomProps, environment: WidgetEnvironment) => {
  'widget';
  const C = props.theme[environment.colorScheme ?? 'light'];
  const family = environment.widgetFamily ?? 'systemSmall';
  const lockScreen = family === 'accessoryCircular' || family === 'accessoryRectangular' || family === 'accessoryInline';
  const mono = lockScreen || (environment.widgetRenderingMode ?? 'fullColor') !== 'fullColor';
  // Extension LTR çizer (`CFBundleLocalizations` yok): Arapçada kenarlar elle çevrilir.
  const edge = props.rtl ? 'trailing' : 'leading';
  const homeRoot: Mods = [
    frame({ maxWidth: Infinity, maxHeight: Infinity }),
    background(C.card),
    containerBackground(C.card, 'widget'),
    widgetURL(props.url),
  ];

  function row(options: { spacing?: number; modifiers?: Mods }, ...children: ReactNode[]) {
    return (
      <HStack spacing={options.spacing ?? 0} alignment="center" modifiers={options.modifiers}>
        {props.rtl ? children.slice().reverse() : children}
      </HStack>
    );
  }

  function label(text: string, size: number, weight: Weight, color: string, lines = 1, extra: Mods = []) {
    return (
      <Text
        modifiers={[
          font({ size, weight }),
          foregroundStyle(color),
          lineLimit(lines),
          minimumScaleFactor(0.8),
          multilineTextAlignment(edge),
        ].concat(extra)}>
        {text}
      </Text>
    );
  }

  function mark(width = 12, height = 20) {
    return (
      <Image uiImage={mono ? props.markMonoUri : props.markUri} modifiers={[resizable(), frame({ width, height })]} />
    );
  }

  function header(text: string) {
    return row({ spacing: 6 }, mark(), label(text, props.rtl ? 13 : 12, 'semibold', C.mfg));
  }

  function stateCard(card: WidgetCard | null | undefined) {
    return (
      <VStack alignment={edge} spacing={6} modifiers={[padding({ all: 14 })].concat(homeRoot)}>
        {header(card?.eyebrow ?? '')}
        {card?.title ? label(card.title, 14, 'semibold', C.fg, 2) : null}
        {card?.body ? label(card.body, 12, 'regular', C.mfg, family === 'systemSmall' ? 3 : 4) : null}
        <Spacer />
        {card?.cta
          ? label(card.cta, 12, 'semibold', C.prifg, 1, [
              padding({ vertical: 5, horizontal: 12 }),
              background(C.pri),
              clipShape('capsule'),
            ])
          : null}
      </VStack>
    );
  }

  // Kilit ekranı: renk verilmez, sistem tonlar. Yalnız `locked` ve durum kartının başlığı okunur.
  const lockRoot: Mods = [containerBackground('transparent', 'widget'), widgetURL(props.url)];

  function lockLabel(text: string, size: number, weight: Weight, lines = 1) {
    return (
      <Text
        modifiers={[font({ size, weight }), lineLimit(lines), minimumScaleFactor(0.7), multilineTextAlignment(edge)]}>
        {text}
      </Text>
    );
  }

  function lockCircular(count: string | undefined, countLabel: string | undefined) {
    return (
      <ZStack modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity })].concat(lockRoot)}>
        <AccessoryWidgetBackground />
        {count ? (
          <VStack alignment="center" spacing={0}>
            {mark(8, 13)}
            {lockLabel(count, 22, 'bold')}
            {countLabel ? lockLabel(countLabel, 11, 'semibold') : null}
          </VStack>
        ) : (
          mark(27, 44)
        )}
      </ZStack>
    );
  }

  function lockRectangular(title: string, main: string, sub: string | undefined) {
    return (
      <VStack
        alignment={edge}
        spacing={2}
        modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity, alignment: edge })].concat(lockRoot)}>
        {row({ spacing: 4 }, mark(9, 15), title ? lockLabel(title, 12, 'semibold') : null)}
        {lockLabel(main, title ? 18 : 14, 'semibold', title ? 1 : 2)}
        {sub ? lockLabel(sub, 12, 'regular') : null}
      </VStack>
    );
  }

  function lockInline(text: string) {
    // WidgetKit satır içinde yalnız bir `Image` ve `Text` çizer; boyut ve renk yok sayılır.
    return (
      <HStack spacing={6} modifiers={lockRoot}>
        {mark(10, 17)}
        <Text>{text}</Text>
      </HStack>
    );
  }

  if (lockScreen) {
    const copy = props.state === 'ok' ? props.locked : undefined;
    const fallback = props.card?.title ?? '';
    if (family === 'accessoryCircular') return lockCircular(copy?.count, copy?.countLabel);
    if (family === 'accessoryRectangular') return lockRectangular(copy?.title ?? '', copy?.main ?? fallback, copy?.sub);
    return lockInline(copy?.inline ?? fallback);
  }

  if (props.state !== 'ok') return stateCard(props.card);

  // İskelet: yalnız sayılar.
  return (
    <VStack alignment={edge} spacing={0} modifiers={[padding({ all: 14 })].concat(homeRoot)}>
      {header(props.title)}
      <Spacer />
      {label(props.locked.main, 18, 'semibold', C.fg, 1, [monospacedDigit()])}
      {label(props.locked.sub, 12, 'regular', C.mfg, 1, [monospacedDigit()])}
      <Spacer />
      {props.staleLabel ? label(props.staleLabel, 11, 'regular', C.mfg, 2) : label(props.updatedLabel, 11, 'regular', C.mfg)}
    </VStack>
  );
};

/** Her açılışta layout'u saklar; `initialProps` ilk eşitlemeye kadar çizilen giriş kartıdır. */
export const create = (initialProps: BaseProps) =>
  createWidget<PanomProps>('OlexPanom', PanomLayout, initialProps as PanomProps);
