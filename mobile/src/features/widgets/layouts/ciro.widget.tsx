// Ciro widget'ı. `'widget'` gövdesi Babel ile metne çevrilip extension'da çalışır (`widgets-plugin.js`):
// dış kapsama erişemez, yardımcılar gövdededir (`koruma.widget.tsx`'in bilinçli kopyası), her metin ve renk `props`'tan gelir.
// `state !== 'ok'` iken `props` yalnız BaseProps'tur: bu dallar `card` dışında veri okumaz.
// Tutar varsayılan gizlidir (`revealed: false`); `Button` değeri `props` üzerinden çevirir, uygulama süreci gerekmez.
// Dürüst sınır: tutar ortak alanda düz metin durur; gizlilik omuz üstü bakışa karşıdır. Kilit ekranı ailesi yok.
import type { CommonViewModifierProps } from '@expo/ui/swift-ui';
import { Button, HStack, Image, Spacer, Text, VStack } from '@expo/ui/swift-ui';
import {
  background,
  buttonStyle,
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
  padding,
  resizable,
  widgetURL,
} from '@expo/ui/swift-ui/modifiers';
import type { WidgetEnvironment } from 'expo-widgets';
import { createWidget } from 'expo-widgets';
import type { ReactElement, ReactNode } from 'react';

import type { BaseProps, RevenueProps, WidgetCard } from '../types';

type Mods = NonNullable<CommonViewModifierProps['modifiers']>;
type Weight = 'regular' | 'semibold' | 'bold';

const CiroLayout = (props: RevenueProps, environment: WidgetEnvironment) => {
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

  const shown = props.revealed === true;

  /** iOS 17+: dokunuş `props.revealed` değerini çevirir. iOS 16'da düğme eylemsizdir; kök `widgetURL` uygulamayı açar. */
  function toggle(content: ReactElement) {
    return (
      <Button
        target="revenue-toggle"
        onPress={() => ({ ...props, revealed: !props.revealed })}
        modifiers={[buttonStyle('plain'), frame({ maxWidth: Infinity, maxHeight: Infinity, alignment: edge })]}>
        {content}
      </Button>
    );
  }

  function small() {
    return (
      <VStack alignment={edge} spacing={0} modifiers={[padding({ all: 14 })].concat(homeRoot)}>
        {header(props.monthTitle)}
        {toggle(
          <VStack alignment={edge} spacing={0}>
            <Spacer />
            {shown
              ? label(props.incomeLabel, props.rtl ? 22 : 26, 'bold', C.fg, 1, [monospacedDigit()])
              : label(props.maskedLabel, 28, 'bold', C.fg, 1, [kerning(2)])}
            {label(shown ? [props.netTitle, props.netLabel].join(' ') : props.tapToReveal, 12, 'regular', C.mfg, 1, [
              monospacedDigit(),
            ])}
            <Spacer />
            {props.staleLabel
              ? label(props.staleLabel, 11, 'regular', C.mfg, 2)
              : shown
              ? label(props.updatedLabel, 11, 'regular', C.mfg, 1, [monospacedDigit()])
              : row(
                  { spacing: 6 },
                  <Image systemName="eye.slash" size={13} color={C.mfg} />,
                  label(props.hiddenTitle, 11, 'regular', C.mfg),
                )}
          </VStack>,
        )}
      </VStack>
    );
  }

  function statTile(title: string, value: string, fill: string) {
    return (
      <VStack
        alignment={edge}
        spacing={0}
        modifiers={[
          padding({ vertical: 7, horizontal: 10 }),
          frame({ maxWidth: Infinity, alignment: edge }),
          background(fill),
          clipShape('roundedRectangle', 12),
        ]}>
        {label(title, 11, 'regular', C.mfg)}
        {shown
          ? label(value, 15, 'semibold', C.fg, 1, [monospacedDigit()])
          : label(props.maskedLabel, 15, 'semibold', C.fg, 1, [kerning(1)])}
      </VStack>
    );
  }

  function medium() {
    return (
      <VStack alignment={edge} spacing={0} modifiers={[padding({ vertical: 14, horizontal: 16 })].concat(homeRoot)}>
        {toggle(
          <VStack alignment={edge} spacing={0}>
            {row({}, header(props.monthTitle), <Spacer />, label(props.staleLabel ?? props.updatedLabel, 11, 'regular', C.mfg, 1, [monospacedDigit()]))}
            {shown
              ? label(props.incomeLabel, props.rtl ? 26 : 30, 'bold', C.fg, 1, [monospacedDigit()])
              : label(props.maskedLabel, 30, 'bold', C.fg, 1, [kerning(2)])}
            {shown ? null : label(props.tapToReveal, 12, 'regular', C.mfg)}
            <Spacer />
            {row(
              { spacing: 8 },
              statTile(props.expenseTitle, props.expenseLabel, C.muted),
              statTile(props.netTitle, props.netLabel, C.sec),
            )}
          </VStack>,
        )}
      </VStack>
    );
  }

  if (props.state !== 'ok') return stateCard(props.card);

  // Büyük boy galeride yok; gelirse orta düzen çizilir.
  return family === 'systemSmall' ? small() : medium();
};

/** Her açılışta layout'u saklar; `initialProps` ilk eşitlemeye kadar çizilen giriş kartıdır. */
export const create = (initialProps: BaseProps) =>
  createWidget<RevenueProps>('OlexCiro', CiroLayout, initialProps as RevenueProps);
