// Panom widget'ı: küçük, orta, büyük ana ekran ve kilit ekranı. Müşteri adı hiçbir yerde yok.
// `'widget'` gövdesi metne çevrilip extension'da çalışır; yardımcılar bilinçli olarak `koruma.widget.tsx`'in kopyasıdır.
// `state !== 'ok'` iken `props` yalnız BaseProps'tur: bu dallar `card` dışında veri okumaz.
import type { CommonViewModifierProps } from '@expo/ui/swift-ui';
import { AccessoryWidgetBackground, HStack, Image, Link, Spacer, Text, VStack, ZStack } from '@expo/ui/swift-ui';
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
  padding,
  resizable,
  strokeBorder,
  widgetURL,
} from '@expo/ui/swift-ui/modifiers';
import type { WidgetEnvironment } from 'expo-widgets';
import { createWidget } from 'expo-widgets';
import type { ComponentProps, ReactNode } from 'react';

import type { BaseProps, PanomProps, PanomRow, WidgetCard } from '../types';

type Mods = NonNullable<CommonViewModifierProps['modifiers']>;
type Weight = 'regular' | 'semibold' | 'bold';
type Symbol = NonNullable<ComponentProps<typeof Image>['systemName']>;

const PanomLayout = (props: PanomProps, environment: WidgetEnvironment) => {
  'widget';
  const C = props.theme[environment.colorScheme ?? 'light'];
  const family = environment.widgetFamily ?? 'systemSmall';
  const lockScreen = family === 'accessoryCircular' || family === 'accessoryRectangular' || family === 'accessoryInline';
  const mono = lockScreen || (environment.widgetRenderingMode ?? 'fullColor') !== 'fullColor';
  // Extension LTR çizer (`CFBundleLocalizations` yok): Arapçada kenarlar elle çevrilir.
  const edge = props.rtl ? 'trailing' : 'leading';
  const rootTo = (url: string): Mods => [
    frame({ maxWidth: Infinity, maxHeight: Infinity, alignment: edge }),
    background(C.card),
    containerBackground(C.card, 'widget'),
    widgetURL(url),
  ];
  const homeRoot = rootTo(props.url);

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
    const button = card?.cta
      ? label(card.cta, 12, 'semibold', C.prifg, 1, [
          padding({ vertical: 5, horizontal: 12 }),
          background(C.pri),
          clipShape('capsule'),
        ])
      : null;
    // Küçük boyda `Link` çalışmaz: tüm widget düğmenin adresini açar.
    const small = family === 'systemSmall';
    const url = small && card?.ctaUrl ? card.ctaUrl : props.url;
    return (
      <VStack alignment={edge} spacing={6} modifiers={[padding({ all: 14 })].concat(rootTo(url))}>
        {header(card?.eyebrow ?? '')}
        {card?.title ? label(card.title, 14, 'semibold', C.fg, 2) : null}
        {card?.body ? label(card.body, 12, 'regular', C.mfg, family === 'systemSmall' ? 3 : 4) : null}
        <Spacer />
        {button && card?.ctaUrl && !small ? <Link destination={card.ctaUrl}>{button}</Link> : button}
      </VStack>
    );
  }

  /** Plaka, araç adı: Latin ve LTR, Arapçada da sola yaslı. */
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

  function icon(name: string, size: number, color: string) {
    return <Image systemName={name as Symbol} size={size} color={color} />;
  }

  function toneColor(tone: PanomRow['statusTone'] | undefined) {
    if (tone === 'done') return C.ok;
    if (tone === 'progress' || tone === 'problem') return C.warn;
    return C.mfg;
  }

  function plateBadge(plate: string, country: string | undefined) {
    // Sıra çevrilmez: plaka Arapçada da LTR kalır.
    return (
      <HStack
        spacing={0}
        modifiers={[
          background(C.plate),
          clipShape('roundedRectangle', 4),
          strokeBorder({ content: props.theme.light.bd, style: { lineWidth: 1 }, shape: 'roundedRectangle', cornerRadius: 4 }),
        ]}>
        {country ? (
          <Text
            modifiers={[
              font({ size: 8, weight: 'bold' }),
              foregroundStyle(C.plate),
              padding({ vertical: 3, horizontal: 3 }),
              background(C.plateStrip),
            ]}>
            {country}
          </Text>
        ) : null}
        <Text
          modifiers={[
            font({ size: 11, weight: 'bold' }),
            foregroundStyle(C.plateFg),
            kerning(0.5),
            lineLimit(1),
            padding({ vertical: 2, horizontal: 4 }),
          ]}>
          {plate}
        </Text>
      </HStack>
    );
  }

  function logoTile(r: PanomRow) {
    return (
      <ZStack
        modifiers={[
          frame({ width: 36, height: 36 }),
          strokeBorder({ content: C.bd, style: { lineWidth: 1 }, shape: 'roundedRectangle', cornerRadius: 10 }),
        ]}>
        {r.logoUri ? (
          <Image
            uiImage={r.logoUri}
            modifiers={[resizable(), aspectRatio({ contentMode: 'fit' }), frame({ width: 26, height: 26 })]}
          />
        ) : (
          label(r.initial ?? '', 15, 'semibold', C.pri, 1, [multilineTextAlignment('center')])
        )}
      </ZStack>
    );
  }

  function queueRow(r: PanomRow) {
    return (
      <Link destination={r.url}>
        {row(
          {
            spacing: 10,
            modifiers: [
              padding({ horizontal: 10 }),
              frame({ height: 56 }),
              frame({ maxWidth: Infinity }),
              strokeBorder({ content: C.bd, style: { lineWidth: 1 }, shape: 'roundedRectangle', cornerRadius: 14 }),
            ],
          },
          logoTile(r),
          <VStack alignment="leading" spacing={3} modifiers={[frame({ maxWidth: Infinity, alignment: 'leading' })]}>
            {latin(r.carTitle ?? '', 14, 'semibold', C.fg)}
            <HStack spacing={6} alignment="center">
              {r.plate ? plateBadge(r.plate, r.plateCountry) : null}
              {r.meta ? label(r.meta, 11, 'regular', C.mfg, 1, [frame({ maxWidth: Infinity, alignment: edge })]) : null}
            </HStack>
          </VStack>,
          icon(r.statusIcon ?? 'clock', 14, toneColor(r.statusTone)),
        )}
      </Link>
    );
  }

  function chip(symbol: string, value: string, caption: string, fill: string, primary: boolean) {
    return (
      <VStack
        alignment={edge}
        spacing={0}
        modifiers={[
          padding({ vertical: 6, horizontal: 9 }),
          frame({ maxWidth: Infinity, alignment: edge }),
          background(fill),
          clipShape('roundedRectangle', 12),
        ]}>
        {row(
          { spacing: 6 },
          <ZStack modifiers={[frame({ width: 22, height: 22 }), background(C.card), clipShape('circle')]}>{icon(symbol, 12, C.pri)}</ZStack>,
          label(value, 24, 'bold', primary ? C.pri : C.fg, 1, [monospacedDigit()]),
        )}
        {label(caption, 11, 'semibold', primary ? C.fg : C.mfg)}
      </VStack>
    );
  }

  /** Simge + "İşlemde 5"; küçük ve büyük boy üst bilgi. */
  function stat(symbol: string, caption: string, value: string, size: number) {
    return row(
      { spacing: 3 },
      icon(symbol, size, C.mfg),
      label(`${caption} ${value}`, size, 'regular', C.mfg, 1, [monospacedDigit()]),
    );
  }

  /** Bayat veri: `ok` içeriğinin üstünde bir satır. */
  function staleLine(lines = 1) {
    return props.staleLabel ? label(props.staleLabel, 11, 'regular', C.mfg, lines) : null;
  }

  /** Simge, ad, sağa yaslı değer: dar boyda da değer kesilmez. */
  function statLine(symbol: string, caption: string, value: string) {
    return row(
      { spacing: 4, modifiers: [frame({ maxWidth: Infinity })] },
      icon(symbol, 11, C.mfg),
      label(caption, 11, 'regular', C.mfg),
      <Spacer />,
      label(value, 12, 'semibold', C.fg, 1, [monospacedDigit()]),
    );
  }

  function small() {
    // Sıra boşsa asıl iş işlemdekilerdir: büyük sayı onu gösterir.
    const idle = props.queueEmpty;
    return (
      <VStack alignment={edge} spacing={0} modifiers={[padding({ all: 14 })].concat(homeRoot)}>
        {header(props.title)}
        <Spacer />
        {row(
          { spacing: 8 },
          <ZStack modifiers={[frame({ width: 30, height: 30 }), background(C.sec), clipShape('circle')]}>
            {icon(idle ? 'wrench.and.screwdriver' : 'clock', 16, C.pri)}
          </ZStack>,
          label(idle ? props.active : props.next, 40, 'bold', C.pri, 1, [monospacedDigit()]),
        )}
        {label(idle ? props.labels.active : props.labels.nextAction, 13, 'semibold', C.fg)}
        <Spacer />
        <VStack alignment={edge} spacing={2}>
          {idle
            ? statLine('clock', props.labels.next, props.next)
            : statLine('wrench.and.screwdriver', props.labels.active, props.active)}
          {statLine('checkmark.circle', props.labels.doneToday, props.done)}
        </VStack>
        {staleLine(2)}
      </VStack>
    );
  }

  function medium() {
    const longest = props.longest;
    return (
      <VStack alignment={edge} spacing={4} modifiers={[padding({ all: 12 })].concat(homeRoot)}>
        {row({ spacing: 6 }, header(props.title), <Spacer />, staleLine())}
        <Spacer />
        {row(
          { spacing: 8 },
          chip('clock', props.next, props.labels.next, C.sec, true),
          chip('wrench.and.screwdriver', props.active, props.labels.active, C.muted, false),
          chip('checkmark.circle', props.done, props.labels.doneToday, C.muted, false),
        )}
        <Spacer />
        {longest ? (
          <Link destination={longest.url}>
            {row(
              { spacing: 8, modifiers: [padding({ horizontal: 4 }), frame({ maxWidth: Infinity })] },
              longest.heroUri ? (
                <Image
                  uiImage={longest.heroUri}
                  modifiers={[resizable(), aspectRatio({ contentMode: 'fit' }), frame({ width: 50, height: 32 })]}
                />
              ) : null,
              <VStack alignment="leading" spacing={0} modifiers={[frame({ maxWidth: Infinity, alignment: 'leading' })]}>
                {label(longest.waitLabel, 11, 'regular', C.mfg, 1, [frame({ maxWidth: Infinity, alignment: edge })])}
                {latin(longest.plate ? `${longest.carTitle} · ${longest.plate}` : longest.carTitle, 14, 'semibold', C.fg)}
              </VStack>,
            )}
          </Link>
        ) : null}
      </VStack>
    );
  }

  function large() {
    return (
      <VStack alignment={edge} spacing={8} modifiers={[padding({ all: 14 })].concat(homeRoot)}>
        {row(
          { spacing: 6 },
          mark(),
          label(props.title, 18, 'semibold', C.fg),
          <Spacer />,
          props.queueEmpty
            ? stat('clock', props.labels.next, props.next, 12)
            : stat('wrench.and.screwdriver', props.labels.active, props.active, 12),
          stat('checkmark.circle', props.labels.doneToday, props.done, 12),
        )}
        {staleLine()}
        {row(
          { spacing: 8 },
          icon(props.queueEmpty ? 'wrench.and.screwdriver' : 'clock', 15, C.pri),
          label(props.queueEmpty ? props.labels.active : props.labels.queue, 13, 'semibold', C.fg),
          label(props.queueEmpty ? props.active : props.next, 12, 'semibold', C.pri, 1, [
            monospacedDigit(),
            padding({ horizontal: 7, vertical: 1 }),
            background(C.sec),
            clipShape('capsule'),
          ]),
          <Spacer />,
        )}
        <VStack alignment="leading" spacing={6}>
          {(props.rows ?? []).slice(0, 4).map((r, i) => (
            <VStack key={i} alignment="leading" spacing={0}>
              {queueRow(r)}
            </VStack>
          ))}
        </VStack>
        <Spacer />
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

  if (family === 'systemMedium') return medium();
  if (family === 'systemLarge') return large();
  return small();
};

/** Her açılışta layout'u saklar; `initialProps` ilk eşitlemeye kadar çizilen giriş kartıdır. */
export const create = (initialProps: BaseProps) =>
  createWidget<PanomProps>('OlexPanom', PanomLayout, initialProps as PanomProps);
