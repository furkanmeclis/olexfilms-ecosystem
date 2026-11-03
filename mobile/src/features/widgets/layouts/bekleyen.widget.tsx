// Bekleyenler widget'ı. `'widget'` gövdesi Babel ile metne çevrilip extension'da çalışır (`widgets-plugin.js`):
// dış kapsama erişemez, yardımcılar gövdededir (`koruma.widget.tsx`'in bilinçli kopyası), her metin ve renk `props`'tan gelir.
// `state !== 'ok'` iken `props` yalnız BaseProps'tur: bu dallar `card` dışında veri okumaz.
// Bu bir onay kuyruğu değildir: üç sayı mevcut uçlardan türer, her satır ilgili liste ekranını açar.
import type { CommonViewModifierProps } from '@expo/ui/swift-ui';
import { HStack, Image, Link, Spacer, Text, VStack, ZStack } from '@expo/ui/swift-ui';
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

import type { BaseProps, PendingProps, PendingRow, WidgetCard } from '../types';

type Mods = NonNullable<CommonViewModifierProps['modifiers']>;
type Weight = 'regular' | 'semibold' | 'bold';
type Symbol = NonNullable<ComponentProps<typeof Image>['systemName']>;

const BekleyenLayout = (props: PendingProps, environment: WidgetEnvironment) => {
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

  function icon(id: PendingRow['id']): Symbol {
    if (id === 'ready') return 'car';
    if (id === 'claim') return 'exclamationmark.shield';
    return 'clock.badge.exclamationmark';
  }

  /** Bayat veri: `ok` içeriğinin üstünde bir satır. */
  function staleLine(lines = 1) {
    return props.staleLabel ? label(props.staleLabel, 11, 'regular', C.mfg, lines) : null;
  }

  function smallRow(r: PendingRow) {
    return row(
      { spacing: 6, modifiers: [frame({ height: 40 })] },
      badge(icon(r.id), 22),
      label(r.label, 12, 'semibold', C.fg, 2, [frame({ maxWidth: Infinity, alignment: edge })]),
      label(r.count, 20, 'bold', C.fg, 1, [monospacedDigit()]),
    );
  }

  function small(rows: PendingRow[]) {
    return (
      <VStack alignment={edge} spacing={0} modifiers={[padding({ all: 14 })].concat(homeRoot)}>
        {header(props.title)}
        {staleLine(2)}
        <Spacer />
        <VStack spacing={0}>{rows.slice(0, 2).map((r) => smallRow(r))}</VStack>
      </VStack>
    );
  }

  function mediumRow(r: PendingRow) {
    return (
      <Link destination={r.url} modifiers={[frame({ maxWidth: Infinity, maxHeight: 36 })]}>
        {row(
          { spacing: 9 },
          badge(icon(r.id), 26),
          <VStack alignment={edge} spacing={0} modifiers={[frame({ maxWidth: Infinity, alignment: edge })]}>
            {label(r.label, 14, 'semibold', C.fg)}
            {r.detail ? label(r.detail, 11, 'regular', C.mfg) : null}
          </VStack>,
          label(r.count, 22, 'bold', C.fg, 1, [monospacedDigit()]),
        )}
      </Link>
    );
  }

  function medium(rows: PendingRow[]) {
    return (
      <VStack alignment={edge} spacing={0} modifiers={[padding({ vertical: 10, horizontal: 14 })].concat(homeRoot)}>
        {row({ spacing: 6 }, header(props.title), <Spacer />, staleLine())}
        <Spacer />
        <VStack spacing={8}>{rows.slice(0, 3).map((r) => mediumRow(r))}</VStack>
        <Spacer />
      </VStack>
    );
  }

  if (props.state !== 'ok') return stateCard(props.card);

  const rows = props.rows ?? [];
  if (rows.length === 0) return stateCard({ eyebrow: props.title, title: props.title });
  // Büyük boy galeride yok; gelirse orta düzen çizilir.
  if (family === 'systemSmall') return small(rows);
  return medium(rows);
};

/** Her açılışta layout'u saklar; `initialProps` ilk eşitlemeye kadar çizilen giriş kartıdır. */
export const create = (initialProps: BaseProps) =>
  createWidget<PendingProps>('OlexBekleyenler', BekleyenLayout, initialProps as PendingProps);
