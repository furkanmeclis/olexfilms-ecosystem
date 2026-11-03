// Widget katmanının ortak sözleşmesi. Saf dosya: yalnız tip.

export type WidgetState =
  | 'ok'
  | 'signed_out'
  | 'empty'
  | 'no_permission'
  | 'module_off'
  | 'wrong_role';

/** Tek bir tema için renk jetonları (`#rrggbb`). */
export type Palette = {
  card: string;
  fg: string;
  pri: string;
  prifg: string;
  sec: string;
  muted: string;
  track: string;
  mfg: string;
  bd: string;
  warn: string;
  warnbg: string;
  ok: string;
  green: string;
  onGreen: string;
  onGreenMuted: string;
  amber: string;
  plate: string;
  plateStrip: string;
};

export type WidgetTheme = { light: Palette; dark: Palette };

/** `state !== 'ok'` iken gösterilen bilgi kartı. */
export type WidgetCard = {
  eyebrow: string;
  title: string;
  body?: string;
  cta?: string;
};

export type BaseProps = {
  state: WidgetState;
  /** Dil Arapça ise true; yatay satırlar ters çizilir. */
  rtl: boolean;
  theme: WidgetTheme;
  /** Olex işareti (ana ekran) ve beyaz tek renkli sürüm (kilit ekranı), `file://`. */
  markUri: string;
  markMonoUri: string;
  /** Widget'a dokununca açılan uygulama içi bağlantı. */
  url: string;
  updatedLabel: string;
  /** Veri bayatsa gösterilen metin, değilse null. */
  staleLabel: string | null;
  card?: WidgetCard;
};
