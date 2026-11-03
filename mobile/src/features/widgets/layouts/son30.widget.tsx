// Son 30 gün widget'ı. `'widget'` gövdesi Babel ile metne çevrilip extension'da çalışır (`widgets-plugin.js`):
// dış kapsama erişemez, yardımcılar gövdededir (`koruma.widget.tsx`'in bilinçli kopyası), her metin ve renk `props`'tan gelir.
// `state !== 'ok'` iken `props` yalnız BaseProps'tur: bu dallar `card` dışında veri okumaz.
import type { CommonViewModifierProps } from '@expo/ui/swift-ui';
import { HStack, Image, RoundedRectangle, Spacer, Text, VStack, ZStack } from '@expo/ui/swift-ui';
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
import type { ComponentProps, ReactNode } from 'react';

import type { BaseProps, KpiCard, KpiProps, WidgetCard } from '../types';

type Mods = NonNullable<CommonViewModifierProps['modifiers']>;
type Weight = 'regular' | 'semibold' | 'bold';
type Symbol = NonNullable<ComponentProps<typeof Image>['systemName']>;

const Son30Layout = (props: KpiProps, environment: WidgetEnvironment) => {
  'widget';
  const C = props.theme[environment.colorScheme ?? 'light'];
  const family = environment.widgetFamily ?? 'systemSmall';
  const mono = (environment.widgetRenderingMode ?? 'fullColor') !== 'fullColor';
  // Extension LTR çizer (`CFBundleLocalizations` yok): Arapçada kenarlar elle çevrilir.
  const edge = props.rtl ? 'trailing' : 'leading';
  const homeRoot: Mods = [
    frame({ maxWidth: Infinity, maxHeight: Infinity, alignment: edge }),
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

  /** Yuvarlak zemin üstünde SF Symbol. */
  function badge(symbol: Symbol, size: number) {
    return (
      <ZStack modifiers={[frame({ width: size, height: size }), background(C.sec), clipShape('circle')]}>
        <Image systemName={symbol} size={size * 0.55} color={C.pri} />
      </ZStack>
    );
  }

  function icon(key: string): Symbol {
    if (key === 'services_created') return 'plus.circle';
    if (key === 'services_completed') return 'checkmark.circle';
    if (key === 'services_open') return 'wrench.and.screwdriver';
    if (key === 'warranties_expiring') return 'exclamationmark.shield';
    return 'shield';
  }

  function tile(card: KpiCard) {
    return row(
      {
        spacing: 8,
        modifiers: [
          padding({ vertical: 4, horizontal: 8 }),
          frame({ maxWidth: Infinity, maxHeight: Infinity, alignment: edge }),
          background(C.muted),
          clipShape('roundedRectangle', 12),
        ],
      },
      badge(icon(card.key), 24),
      <VStack alignment={edge} spacing={0}>
        {label(card.value, 20, 'bold', C.fg, 1, [monospacedDigit()])}
        {label(card.label, 11, 'semibold', C.mfg)}
      </VStack>,
      <Spacer />,
    );
  }

  /** Kartlar ikişerli satırlara bölünür; eksik kart (rol, modül) boşluk bırakmaz. */
  function grid(cards: KpiCard[], gap: number) {
    const lines: KpiCard[][] = [];
    for (let i = 0; i < cards.length; i += 2) lines.push(cards.slice(i, i + 2));
    return (
      <VStack spacing={gap} modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity })]}>
        {lines.map((line, i) => (
          <HStack key={i} spacing={gap} modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity })]}>
            {(props.rtl ? line.slice().reverse() : line).map((card) => (
              <HStack key={card.key} spacing={0} modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity })]}>
                {tile(card)}
              </HStack>
            ))}
            {line.length < 2 ? <Spacer /> : null}
          </HStack>
        ))}
      </VStack>
    );
  }

  /** Bayat veri: `ok` içeriğinin üstünde bir satır. */
  function staleLine(lines = 1) {
    return props.staleLabel ? label(props.staleLabel, 11, 'regular', C.mfg, lines) : null;
  }

  function small(primary: KpiCard) {
    return (
      <VStack alignment={edge} spacing={0} modifiers={[padding({ all: 14 })].concat(homeRoot)}>
        {header(props.title)}
        <Spacer />
        <VStack alignment={edge} spacing={2}>
          {row({ spacing: 8 }, badge(icon(primary.key), 30), label(primary.value, 40, 'bold', C.fg, 1, [monospacedDigit()]))}
          {label(primary.label, 13, 'semibold', C.fg, 2)}
        </VStack>
        <Spacer />
        {props.staleLabel
          ? staleLine(2)
          : props.summary
            ? label(props.summary, 12, 'regular', C.mfg, 1, [monospacedDigit()])
            : null}
      </VStack>
    );
  }

  function medium(cards: KpiCard[]) {
    return (
      <VStack alignment={edge} spacing={6} modifiers={[padding({ all: 12 })].concat(homeRoot)}>
        {row({ spacing: 6 }, header(props.title), <Spacer />, staleLine())}
        {grid(cards, 8)}
      </VStack>
    );
  }

  function weekly() {
    const bars = props.bars ?? [];
    return (
      <VStack alignment={edge} spacing={6} modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity })]}>
        {label(props.weeklyTitle, 12, 'semibold', C.fg)}
        <HStack
          spacing={10}
          alignment="bottom"
          modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity, alignment: 'bottom' })]}>
          {(props.rtl ? bars.slice().reverse() : bars).map((bar, i) => (
            <VStack key={i} alignment="center" spacing={3} modifiers={[frame({ maxWidth: Infinity })]}>
              {label(bar.value, 11, 'semibold', C.fg, 1, [monospacedDigit()])}
              <RoundedRectangle
                cornerRadius={6}
                modifiers={[foregroundStyle(bar.current ? C.pri : C.track), frame({ height: Math.max(2, bar.height) })]}
              />
              {label(bar.label, 10, 'regular', C.mfg)}
            </VStack>
          ))}
        </HStack>
      </VStack>
    );
  }

  function large(cards: KpiCard[]) {
    return (
      <VStack alignment={edge} spacing={10} modifiers={[padding({ all: 14 })].concat(homeRoot)}>
        {row({}, header(props.title), <Spacer />, label(props.staleLabel ?? props.updatedLabel, 11, 'regular', C.mfg, 1, [monospacedDigit()]))}
        {grid(cards, 8)}
        {weekly()}
      </VStack>
    );
  }

  if (props.state !== 'ok') return stateCard(props.card);

  const all = props.cards ?? [];
  if (all.length === 0) return stateCard({ eyebrow: props.title, title: props.title });
  // Izgara ikişerli iki satırdır (tasarımda dört kart); beşi birden sığmaz, "Devam" küçük boy özetinde kalır.
  const cards = all.length > 4 ? all.filter((c) => c.key !== 'services_open').slice(0, 4) : all;
  if (family === 'systemMedium') return medium(cards);
  if (family === 'systemLarge') return large(cards);
  return small(props.primary ?? all[0]);
};

/** Her açılışta layout'u saklar; `initialProps` ilk eşitlemeye kadar çizilen giriş kartıdır. */
export const create = (initialProps: BaseProps) =>
  createWidget<KpiProps>('OlexSon30Gun', Son30Layout, initialProps as KpiProps);
