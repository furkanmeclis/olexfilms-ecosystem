// Garanti kartı widget'ı. `'widget'` gövdesi Babel ile metne çevrilip extension'da çalışır (`widgets-plugin.js`):
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
  multilineTextAlignment,
  padding,
  resizable,
  widgetURL,
} from '@expo/ui/swift-ui/modifiers';
import type { WidgetEnvironment } from 'expo-widgets';
import { createWidget } from 'expo-widgets';
import type { ReactNode } from 'react';

import type { BaseProps, KartProps, KartVehicle, WidgetCard } from '../types';

type KartConfig = { vehicle?: string };
type Mods = NonNullable<CommonViewModifierProps['modifiers']>;
type Weight = 'regular' | 'semibold' | 'bold';

const KartLayout = (props: KartProps, environment: WidgetEnvironment<KartConfig>) => {
  'widget';
  const C = props.theme[environment.colorScheme ?? 'light'];
  const family = environment.widgetFamily ?? 'systemSmall';
  const mono = (environment.widgetRenderingMode ?? 'fullColor') !== 'fullColor';
  // Extension LTR çizer (`CFBundleLocalizations` yok): Arapçada kenarlar elle çevrilir.
  const edge = props.rtl ? 'trailing' : 'leading';
  const root: Mods = [
    frame({ maxWidth: Infinity, maxHeight: Infinity }),
    background(C.green),
    containerBackground(C.green, 'widget'),
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

  /** Kod, araç adı, plaka: Latin ve LTR, Arapçada da sola yaslı. */
  function latin(text: string, size: number, color: string, weight: Weight = 'regular') {
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

  /** Genel kod: monospaced, her zaman LTR ve ortalı. */
  function code(text: string, size: number, color: string, weight: Weight = 'regular') {
    return (
      <Text
        modifiers={[
          font({ size, weight, design: 'monospaced' }),
          foregroundStyle(color),
          lineLimit(1),
          minimumScaleFactor(0.7),
          multilineTextAlignment('center'),
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

  function stateCard(card: WidgetCard | null | undefined) {
    return (
      <VStack alignment={edge} spacing={6} modifiers={[padding({ all: 14 })].concat(root, [widgetURL(props.url)])}>
        {row({ spacing: 6 }, mark(), label(card?.eyebrow ?? '', props.rtl ? 13 : 12, 'semibold', C.amber))}
        {card?.title ? label(card.title, 14, 'semibold', C.onGreen, 2) : null}
        {card?.body ? label(card.body, 12, 'regular', C.onGreenMuted, family === 'systemSmall' ? 3 : 4) : null}
        <Spacer />
        {card?.cta
          ? label(card.cta, 12, 'semibold', C.prifg, 1, [
              padding({ vertical: 5, horizontal: 12 }),
              background(C.amber),
              clipShape('capsule'),
            ])
          : null}
      </VStack>
    );
  }

  /** Beyaz zeminde QR; `qrUri` yoksa yalnız kod metni çizilir. */
  function qrTile(v: KartVehicle, size: number, pad: number, radius: number, qr: number, codeSize: number) {
    return (
      <VStack
        alignment="center"
        spacing={2}
        modifiers={[padding({ all: pad }), frame({ width: size, height: size }), background(C.plate), clipShape('roundedRectangle', radius)]}>
        {v.qrUri ? (
          <Image
            uiImage={v.qrUri}
            modifiers={[resizable(), aspectRatio({ contentMode: 'fit' }), frame({ width: qr, height: qr })]}
          />
        ) : (
          <Spacer />
        )}
        {v.qrUri && codeSize > 0 ? code(v.publicCode, codeSize, C.plateFg, 'semibold') : null}
        {v.qrUri ? null : code(v.publicCode, 14, C.plateFg, 'semibold')}
        {v.qrUri ? null : <Spacer />}
      </VStack>
    );
  }

  function small(v: KartVehicle) {
    return (
      <VStack alignment="center" spacing={0} modifiers={[padding({ all: 12 })].concat(root, [widgetURL(v.url)])}>
        {row({ spacing: 6 }, mark(), code(v.publicCode, 11, C.onGreen, 'semibold'), <Spacer />)}
        <Spacer />
        {qrTile(v, 106, 7, 12, 92, 0)}
      </VStack>
    );
  }

  function cover(v: KartVehicle) {
    return (
      <ZStack
        modifiers={[frame({ width: 72, height: 44 }), background(C.plate), clipShape('roundedRectangle', 10)]}>
        {v.heroUri ? (
          <Image
            uiImage={v.heroUri}
            modifiers={[resizable(), aspectRatio({ contentMode: 'fit' }), frame({ width: 66, height: 40 })]}
          />
        ) : null}
      </ZStack>
    );
  }

  function medium(v: KartVehicle) {
    return row(
      { spacing: 14, modifiers: [padding({ all: 14 })].concat(root, [widgetURL(v.url)]) },
      <VStack alignment={edge} spacing={0} modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity })]}>
        {row({ spacing: 8 }, mark(16, 26), label(props.title, 12, 'semibold', C.amber))}
        <Spacer />
        {latin(v.productName, 18, C.onGreen, 'semibold')}
        <Spacer />
        {row(
          { spacing: 8 },
          cover(v),
          <VStack alignment="leading" spacing={0}>
            {latin(v.title, 12, C.onGreenMuted)}
            {v.plate ? latin(v.plate, 12, C.onGreenMuted) : null}
          </VStack>,
        )}
        <Spacer />
        {label(v.untilLabel, 11, 'regular', C.onGreenMuted)}
      </VStack>,
      qrTile(v, 124, 8, 14, 94, 10),
    );
  }

  const selected = props.state === 'ok' ? environment.configuration?.vehicle : undefined;
  const vehicle = props.state === 'ok' ? (props.vehicles.find((v) => v.id === selected) ?? props.vehicles[0]) : undefined;

  if (props.state !== 'ok') return stateCard(props.card);
  if (!vehicle) return stateCard(props.card);

  if (family === 'systemMedium' || family === 'systemLarge') return medium(vehicle);
  return small(vehicle);
};

/** Her açılışta layout'u saklar; `initialProps` ilk eşitlemeye kadar çizilen giriş kartıdır. */
export const create = (initialProps: BaseProps) =>
  createWidget<KartProps, KartConfig>('OlexGarantiKarti', KartLayout, initialProps as KartProps);
