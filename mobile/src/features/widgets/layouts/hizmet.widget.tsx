// Hizmet durumu widget'ı. `'widget'` gövdesi Babel ile metne çevrilip extension'da çalışır (`widgets-plugin.js`):
// dış kapsama erişemez, yardımcılar gövdededir, her metin ve renk `props`'tan gelir.
// `state !== 'ok'` iken `props` yalnız BaseProps'tur: bu dallar `card` dışında veri okumaz.
import type { CommonViewModifierProps } from '@expo/ui/swift-ui';
import { HStack, Image, Spacer, Text, VStack, ZStack } from '@expo/ui/swift-ui';
import {
  aspectRatio,
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
  offset,
  padding,
  resizable,
  widgetURL,
} from '@expo/ui/swift-ui/modifiers';
import type { WidgetEnvironment } from 'expo-widgets';
import { createWidget } from 'expo-widgets';
import type { ReactNode } from 'react';

import type { BaseProps, HizmetProps, HizmetService, HizmetStep, HizmetVehicle, WidgetCard } from '../types';

type HizmetConfig = { vehicle?: string };
type Mods = NonNullable<CommonViewModifierProps['modifiers']>;
type Weight = 'regular' | 'semibold' | 'bold';

const HizmetLayout = (props: HizmetProps, environment: WidgetEnvironment<HizmetConfig>) => {
  'widget';
  const C = props.theme[environment.colorScheme ?? 'light'];
  const family = environment.widgetFamily ?? 'systemSmall';
  const mono = (environment.widgetRenderingMode ?? 'fullColor') !== 'fullColor';
  // Extension LTR çizer (`CFBundleLocalizations` yok): Arapçada kenarlar elle çevrilir.
  const edge = props.rtl ? 'trailing' : 'leading';
  const homeRoot = (url: string): Mods => [
    frame({ maxWidth: Infinity, maxHeight: Infinity }),
    background(C.card),
    containerBackground(C.card, 'widget'),
    widgetURL(url),
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

  /** Araç adı, plaka: Latin ve LTR, Arapçada da sola yaslı. */
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

  function stateCard(card: WidgetCard | null | undefined, url = props.url) {
    return (
      <VStack alignment={edge} spacing={6} modifiers={[padding({ all: 14 })].concat(homeRoot(url))}>
        {row({ spacing: 6 }, mark(), label(card?.eyebrow ?? '', props.rtl ? 13 : 12, 'semibold', C.mfg))}
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

  function phaseColor(step: HizmetStep) {
    if (step.phase === 'done') return C.pri;
    if (step.phase === 'current') return C.warn;
    return C.track;
  }

  function segment(step: HizmetStep) {
    return (
      <ZStack
        modifiers={[
          frame({ maxWidth: Infinity }),
          frame({ height: 5 }),
          background(phaseColor(step)),
          clipShape('capsule'),
        ]}>
        <Spacer />
      </ZStack>
    );
  }

  /** Dört adım çizgisi; Arapçada sıra ters akar. */
  function stepBars(s: HizmetService) {
    return row({ spacing: 4 }, ...s.steps.map((step) => segment(step)));
  }

  function stepColumns(s: HizmetService) {
    return row(
      { spacing: 6 },
      ...s.steps.map((step) => (
        <VStack alignment={edge} spacing={6} modifiers={[frame({ maxWidth: Infinity })]}>
          {segment(step)}
          {label(step.label, 11, step.phase === 'current' ? 'bold' : 'regular', step.phase === 'current' ? C.fg : C.mfg)}
        </VStack>
      )),
    );
  }

  function pill(s: HizmetService) {
    return row(
      { spacing: 6, modifiers: [padding({ vertical: 4, horizontal: 9 }), background(s.done ? C.sec : C.warnbg), clipShape('capsule')] },
      <ZStack modifiers={[frame({ width: 6, height: 6 }), background(s.done ? C.ok : C.warn), clipShape('circle')]}>
        <Spacer />
      </ZStack>,
      label(s.statusLabel, 12, 'semibold', C.fg),
    );
  }

  function hero(uri: string | null | undefined, width: number, height: number, x: number, y: number) {
    return uri ? (
      <Image
        uiImage={uri}
        modifiers={[resizable(), aspectRatio({ contentMode: 'fit' }), frame({ width, height }), offset({ x, y })]}
      />
    ) : null;
  }

  function small(v: HizmetVehicle, s: HizmetService) {
    return (
      <ZStack alignment={props.rtl ? 'topLeading' : 'topTrailing'} modifiers={homeRoot(s.url)}>
        {hero(v.heroUri, 96, 58, props.rtl ? -8 : 8, 34)}
        <VStack alignment={edge} spacing={0} modifiers={[padding({ all: 14 })]}>
          {row({ spacing: 6 }, mark(), pill(s))}
          <Spacer />
          <VStack alignment={edge} spacing={1}>
            {latin(v.title, 20, 'semibold', C.fg)}
            {label(s.dealerName, 12, 'regular', C.mfg)}
          </VStack>
          <Spacer />
          {stepBars(s)}
        </VStack>
      </ZStack>
    );
  }

  function medium(v: HizmetVehicle, s: HizmetService) {
    const heading = v.plate ? `${v.title} · ${v.plate}` : v.title;
    return (
      <ZStack alignment={props.rtl ? 'topLeading' : 'topTrailing'} modifiers={homeRoot(s.url)}>
        {hero(v.heroUri, 92, 56, props.rtl ? 6 : -6, 6)}
        <VStack alignment={edge} spacing={0} modifiers={[padding({ vertical: 14, horizontal: 16 })]}>
          {row(
            { spacing: 8, modifiers: [padding({ [props.rtl ? 'leading' : 'trailing']: 92 })] },
            mark(),
            <VStack alignment="leading" spacing={0}>
              {latin(heading, 14, 'semibold', C.fg)}
              {label(s.dealerName, 12, 'regular', C.mfg)}
            </VStack>,
          )}
          <Spacer />
          {stepColumns(s)}
          <Spacer />
          {row(
            {},
            <Text
              modifiers={[font({ size: 11, design: 'monospaced' }), foregroundStyle(C.mfg), lineLimit(1), minimumScaleFactor(0.8)]}>
              {s.serviceNo}
            </Text>,
            <Spacer />,
            label(props.updatedLabel, 11, 'regular', C.mfg, 1, [monospacedDigit()]),
          )}
        </VStack>
      </ZStack>
    );
  }

  const selected = props.state === 'ok' ? environment.configuration?.vehicle : undefined;
  const vehicle = props.state === 'ok' ? (props.vehicles.find((v) => v.id === selected) ?? props.vehicles[0]) : undefined;

  if (props.state !== 'ok') return stateCard(props.card);
  if (!vehicle) return stateCard(props.card);

  const service = vehicle.service;
  if (!service || service.steps.length === 0) return stateCard(vehicle.emptyCard, vehicle.url);
  if (props.staleLabel) {
    return stateCard({ eyebrow: props.eyebrow, title: service.statusLabel, body: props.staleLabel }, service.url);
  }

  if (family === 'systemMedium' || family === 'systemLarge') return medium(vehicle, service);
  return small(vehicle, service);
};

/** Her açılışta layout'u saklar; `initialProps` ilk eşitlemeye kadar çizilen giriş kartıdır. */
export const create = (initialProps: BaseProps) =>
  createWidget<HizmetProps, HizmetConfig>('OlexHizmetDurumu', HizmetLayout, initialProps as HizmetProps);
