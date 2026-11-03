// Koruma widget'ı. `'widget'` gövdesi Babel ile metne çevrilip extension'da çalışır (`widgets-plugin.js`):
// dış kapsama erişemez, yardımcılar gövdededir, her metin ve renk `props`'tan gelir.
// `state !== 'ok'` iken `props` yalnız BaseProps'tur: bu dallar `card` dışında veri okumaz.
import type { CommonViewModifierProps } from '@expo/ui/swift-ui';
import { AccessoryWidgetBackground, HStack, Image, ProgressView, Spacer, Text, VStack, ZStack } from '@expo/ui/swift-ui';
import {
  aspectRatio,
  background,
  clipShape,
  containerBackground,
  font,
  foregroundStyle,
  frame,
  kerning,
  lineLimit,
  minimumScaleFactor,
  monospacedDigit,
  multilineTextAlignment,
  offset,
  padding,
  progressViewStyle,
  resizable,
  scaleEffect,
  strokeBorder,
  tint,
  widgetURL,
} from '@expo/ui/swift-ui/modifiers';
import type { WidgetEnvironment } from 'expo-widgets';
import { createWidget } from 'expo-widgets';
import type { ReactNode } from 'react';

import type { BaseProps, KorumaProps, KorumaVehicle, KorumaWarranty, WidgetCard } from '../types';

type KorumaConfig = { vehicle?: string };
type Mods = NonNullable<CommonViewModifierProps['modifiers']>;
type Weight = 'regular' | 'semibold' | 'bold';

const KorumaLayout = (props: KorumaProps, environment: WidgetEnvironment<KorumaConfig>) => {
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

  /** Plaka, ürün, araç adı: Latin ve LTR, Arapçada da sola yaslı. */
  function latin(text: string, size: number, weight: Weight, color: string) {
    return (
      <Text
        modifiers={[
          font({ size, weight }),
          foregroundStyle(color),
          lineLimit(1),
          minimumScaleFactor(0.8),
          frame({ maxWidth: Infinity, alignment: 'leading' }),
        ]}>
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

  function updatedRow(left: string) {
    return row(
      {},
      left ? label(left, 11, 'regular', C.mfg) : null,
      <Spacer />,
      label(props.updatedLabel, 11, 'regular', C.mfg, 1, [monospacedDigit()]),
    );
  }

  function bar(w: KorumaWarranty) {
    return (
      <ProgressView
        value={w.percent / 100}
        modifiers={[progressViewStyle('linear'), tint(w.warn ? C.amber : C.pri)].concat(
          props.rtl ? [scaleEffect({ x: -1, y: 1 })] : [],
        )}
      />
    );
  }

  function hero(uri: string | null | undefined, width: number, height: number, shift: { x: number; y?: number }) {
    return uri ? (
      <Image
        uiImage={uri}
        modifiers={[resizable(), aspectRatio({ contentMode: 'fit' }), frame({ width, height }), offset(shift)]}
      />
    ) : null;
  }

  function plateBadge(v: KorumaVehicle) {
    // Sıra çevrilmez: plaka Arapçada da LTR kalır.
    return (
      <HStack
        spacing={0}
        modifiers={[
          background(C.plate),
          clipShape('roundedRectangle', 6),
          strokeBorder({ content: props.theme.light.bd, style: { lineWidth: 1 }, shape: 'roundedRectangle', cornerRadius: 6 }),
        ]}>
        {v.plateCountry ? (
          <Text
            modifiers={[
              font({ size: 9, weight: 'bold' }),
              foregroundStyle(C.plate),
              padding({ vertical: 5, horizontal: 4 }),
              background(C.plateStrip),
            ]}>
            {v.plateCountry}
          </Text>
        ) : null}
        <Text
          modifiers={[
            font({ size: 12, weight: 'bold' }),
            foregroundStyle(C.plateFg),
            kerning(0.8),
            lineLimit(1),
            padding({ vertical: 3, horizontal: 6 }),
          ]}>
          {v.plate}
        </Text>
      </HStack>
    );
  }

  function logoTile(v: KorumaVehicle) {
    return (
      <ZStack
        modifiers={[
          frame({ width: 34, height: 34 }),
          strokeBorder({ content: C.bd, style: { lineWidth: 1 }, shape: 'roundedRectangle', cornerRadius: 10 }),
        ]}>
        {v.brandLogoUri ? (
          <Image
            uiImage={v.brandLogoUri}
            modifiers={[resizable(), aspectRatio({ contentMode: 'fit' }), frame({ width: 24, height: 24 })]}
          />
        ) : (
          label(v.brandInitial, 16, 'semibold', C.pri)
        )}
      </ZStack>
    );
  }

  function small(v: KorumaVehicle, w: KorumaWarranty) {
    return (
      <VStack alignment={edge} spacing={0} modifiers={[padding({ all: 14 })].concat(homeRoot)}>
        {header(props.eyebrow)}
        <Spacer />
        <VStack alignment={edge} spacing={1}>
          {label(w.leftLabel, props.rtl ? 21 : 26, 'bold', C.fg, props.rtl ? 2 : 1, [monospacedDigit()])}
          {label([props.leftCaption, w.name].join(' · '), 12, 'regular', C.mfg)}
        </VStack>
        <Spacer />
        <VStack alignment={edge} spacing={6}>
          {bar(w)}
          {v.plate ? latin(v.plate, 11, 'semibold', C.mfg) : null}
        </VStack>
      </VStack>
    );
  }

  function medium(v: KorumaVehicle) {
    const [first, second] = v.warranties;
    return (
      <ZStack alignment={props.rtl ? 'bottomLeading' : 'bottomTrailing'} modifiers={homeRoot}>
        {row(
          { spacing: 12, modifiers: [padding({ all: 14 })] },
          <VStack alignment={edge} spacing={0} modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity })]}>
            {header(props.eyebrow)}
            <Spacer />
            <VStack alignment={edge} spacing={5}>
              {latin(first.name, 12, 'regular', C.mfg)}
              {label(first.leftLongLabel, props.rtl ? 17 : 18, 'semibold', C.fg, 1, [monospacedDigit()])}
              {bar(first)}
            </VStack>
            {second ? <Spacer /> : null}
            {second ? (
              <VStack alignment={edge} spacing={5}>
                {latin(second.name, 12, 'regular', C.mfg)}
                {label(second.leftLongLabel, 14, 'semibold', C.fg, 1, [monospacedDigit()])}
                {bar(second)}
              </VStack>
            ) : null}
          </VStack>,
          <VStack
            alignment="leading"
            spacing={3}
            modifiers={[frame({ width: 128 }), frame({ maxHeight: Infinity, alignment: 'top' })]}>
            {latin(v.title, 14, 'semibold', C.fg)}
            {v.plate ? latin(v.plate, 11, 'semibold', C.mfg) : null}
          </VStack>,
        )}
        {hero(v.heroUri, 158, 96, props.rtl ? { x: -22, y: -6 } : { x: 14, y: -4 })}
      </ZStack>
    );
  }

  function large(v: KorumaVehicle) {
    return (
      <VStack alignment={edge} spacing={0} modifiers={[padding({ all: 16 })].concat(homeRoot)}>
        {row(
          { spacing: 10 },
          mark(),
          logoTile(v),
          <VStack alignment={edge} spacing={0}>
            {latin(v.title, 15, 'semibold', C.fg)}
            {label(v.activeCountLabel, 12, 'regular', C.mfg)}
          </VStack>,
          v.plate ? plateBadge(v) : null,
        )}
        <Spacer />
        {v.heroUri ? row({}, <Spacer />, hero(v.heroUri, 200, 118, { x: props.rtl ? -16 : 16 })) : null}
        <Spacer />
        <VStack alignment={edge} spacing={9}>
          {v.warranties.map((w, i) => (
            <VStack key={i} alignment={edge} spacing={4}>
              {row(
                { spacing: 8 },
                label(w.name, 13, 'semibold', C.fg),
                <Spacer />,
                label(w.leftLongLabel, 13, 'regular', C.mfg, 1, [monospacedDigit()]),
              )}
              {bar(w)}
            </VStack>
          ))}
        </VStack>
        <Spacer />
        {updatedRow(v.lastServiceLabel)}
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

  function lockCircular(short: string | undefined) {
    return (
      <ZStack modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity })].concat(lockRoot)}>
        <AccessoryWidgetBackground />
        {short ? (
          <VStack alignment="center" spacing={0}>
            {mark(9, 15)}
            {lockLabel(short, 15, 'bold')}
          </VStack>
        ) : (
          mark(27, 44)
        )}
      </ZStack>
    );
  }

  function lockRectangular(title: string, text: string) {
    return (
      <VStack
        alignment={edge}
        spacing={3}
        modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity, alignment: edge })].concat(lockRoot)}>
        {row({ spacing: 4 }, mark(9, 15), title ? lockLabel(title, 12, 'semibold') : null)}
        {lockLabel(text, title ? 18 : 14, 'semibold', title ? 1 : 2)}
      </VStack>
    );
  }

  function lockInline(text: string) {
    // WidgetKit satır içinde yalnız bir `Image` ve `Text` çizer; boyut ve renk yok sayılır.
    return (
      <HStack spacing={4} modifiers={lockRoot}>
        {mark(10, 17)}
        <Text>{text}</Text>
      </HStack>
    );
  }

  function lockFamily(title: string, short: string | undefined, long: string, inline: string) {
    if (family === 'accessoryCircular') return lockCircular(short);
    if (family === 'accessoryRectangular') return lockRectangular(title, long);
    return lockInline(inline);
  }

  const selected = props.state === 'ok' ? environment.configuration?.vehicle : undefined;
  const vehicle = props.state === 'ok' ? (props.vehicles.find((v) => v.id === selected) ?? props.vehicles[0]) : undefined;

  if (lockScreen) {
    const copy = vehicle?.locked;
    return copy
      ? lockFamily(copy.title, copy.short, copy.long, copy.inline)
      : lockFamily('', undefined, props.card?.title ?? '', props.card?.title ?? '');
  }

  if (props.state !== 'ok') return stateCard(props.card);
  if (!vehicle) return stateCard(props.card);

  const shortest = vehicle.warranties[0];
  if (!shortest) return stateCard(vehicle.emptyCard);
  if (props.staleLabel) {
    return stateCard({ eyebrow: props.eyebrow, title: shortest.leftLongLabel, body: props.staleLabel });
  }

  if (family === 'systemMedium') return medium(vehicle);
  if (family === 'systemLarge') return large(vehicle);
  return small(vehicle, shortest);
};

/** Her açılışta layout'u saklar; `initialProps` ilk eşitlemeye kadar çizilen giriş kartıdır. */
export const create = (initialProps: BaseProps) =>
  createWidget<KorumaProps, KorumaConfig>('OlexKoruma', KorumaLayout, initialProps as KorumaProps);
