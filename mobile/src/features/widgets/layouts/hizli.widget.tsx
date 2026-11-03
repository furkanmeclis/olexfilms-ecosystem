// Hızlı işlem widget'ı: küçük (Tara), orta (2x2 düğme) ve kilit ekranı dairesi.
// Düğmeler veriye bağlı değildir: bayat veri satırı çizilmez, `staleLabel` bilinçli olarak okunmaz.
// `'widget'` gövdesi metne çevrilip extension'da çalışır; yardımcılar bilinçli olarak `koruma.widget.tsx`'in kopyasıdır.
// `state !== 'ok'` iken `props` yalnız BaseProps'tur: bu dallar `card` dışında veri okumaz.
import type { CommonViewModifierProps } from '@expo/ui/swift-ui';
import { AccessoryWidgetBackground, HStack, Image, Link, Spacer, Text, VStack, ZStack } from '@expo/ui/swift-ui';
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

import type { BaseProps, QuickAction, QuickProps, WidgetCard } from '../types';

type Mods = NonNullable<CommonViewModifierProps['modifiers']>;
type Weight = 'regular' | 'semibold' | 'bold';
type Symbol = NonNullable<ComponentProps<typeof Image>['systemName']>;

const HizliLayout = (props: QuickProps, environment: WidgetEnvironment) => {
  'widget';
  const C = props.theme[environment.colorScheme ?? 'light'];
  const family = environment.widgetFamily ?? 'systemSmall';
  const lockScreen = family === 'accessoryCircular';
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

  function mark(width = 12, height = 20, light = mono) {
    return (
      <Image uiImage={light ? props.markMonoUri : props.markUri} modifiers={[resizable(), frame({ width, height })]} />
    );
  }

  function header(text: string) {
    return row({ spacing: 6 }, mark(), label(text, props.rtl ? 13 : 12, 'semibold', C.mfg));
  }

  function icon(name: string, size: number, color: string) {
    return <Image systemName={name as Symbol} size={size} color={color} />;
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

  const symbols: Record<string, string> = {
    scan: 'viewfinder',
    new_service: 'plus',
    measure: 'gauge',
    customers: 'person.2',
    services: 'list.bullet',
  };

  function small(scan: QuickAction | undefined) {
    // Koyu yeşil zemin her iki temada; işaret beyaz tek renk sürümdür.
    return (
      <VStack
        alignment={edge}
        spacing={0}
        modifiers={[
          padding({ all: 14 }),
          frame({ maxWidth: Infinity, maxHeight: Infinity }),
          background(C.green),
          containerBackground(C.green, 'widget'),
          widgetURL(scan?.url ?? props.url),
        ]}>
        {mark(12, 20, true)}
        <Spacer />
        {row(
          { spacing: 10 },
          <ZStack modifiers={[frame({ width: 48, height: 48 }), background(C.amber), clipShape('roundedRectangle', 14)]}>
            {icon('viewfinder', 26, C.pri)}
          </ZStack>,
          label(scan?.label ?? '', 26, 'bold', C.onGreen),
        )}
      </VStack>
    );
  }

  function button(action: QuickAction, dark: boolean) {
    const count = action.id === 'customers' ? props.customersCount : undefined;
    return (
      <Link destination={action.url} modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity })]}>
        {row(
          {
            spacing: 10,
            modifiers: [
              padding({ horizontal: 12 }),
              frame({ maxWidth: Infinity, minHeight: 44, maxHeight: Infinity, alignment: edge }),
              background(dark ? C.green : C.sec),
              clipShape('roundedRectangle', 14),
            ],
          },
          icon(symbols[action.id] ?? 'circle', 22, dark ? C.amber : C.pri),
          <VStack alignment={edge} spacing={0} modifiers={[frame({ maxWidth: Infinity, alignment: edge })]}>
            {label(action.label, 14, 'semibold', dark ? C.onGreen : C.fg)}
            {count ? label(count, 11, 'regular', C.mfg, 1, [monospacedDigit()]) : null}
          </VStack>,
        )}
      </Link>
    );
  }

  function medium(actions: QuickAction[]) {
    const cells = actions.slice(0, 4).map((a, i) => button(a, i === 0));
    return (
      <VStack alignment={edge} spacing={6} modifiers={[padding({ all: 12 })].concat(homeRoot)}>
        {header(props.title)}
        <VStack spacing={8} modifiers={[frame({ maxHeight: Infinity })]}>
          {row({ spacing: 8, modifiers: [frame({ maxHeight: Infinity })] }, ...cells.slice(0, 2))}
          {cells.length > 2 ? row({ spacing: 8, modifiers: [frame({ maxHeight: Infinity })] }, ...cells.slice(2, 4)) : null}
        </VStack>
      </VStack>
    );
  }

  // Kilit ekranı: renk verilmez, sistem tonlar.
  function lockCircular(url: string | undefined) {
    return (
      <ZStack modifiers={[frame({ maxWidth: Infinity, maxHeight: Infinity }), containerBackground('transparent', 'widget'), widgetURL(url ?? props.url)]}>
        <AccessoryWidgetBackground />
        {url ? (
          <VStack alignment="center" spacing={2}>
            {mark(8, 13)}
            <Image systemName="viewfinder" size={22} />
          </VStack>
        ) : (
          mark(27, 44)
        )}
      </ZStack>
    );
  }

  const actions = props.state === 'ok' ? (props.actions ?? []) : [];
  const scan = actions.find((a) => a.id === 'scan') ?? actions[0];

  if (lockScreen) return lockCircular(scan?.url);

  if (props.state !== 'ok') return stateCard(props.card);
  if (actions.length === 0) return stateCard(props.card);

  if (family === 'systemMedium') return medium(actions);
  return small(scan);
};

/** Her açılışta layout'u saklar; `initialProps` ilk eşitlemeye kadar çizilen giriş kartıdır. */
export const create = (initialProps: BaseProps) =>
  createWidget<QuickProps>('OlexHizliIslem', HizliLayout, initialProps as QuickProps);
