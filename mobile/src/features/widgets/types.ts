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

// --- Müşteri widget'ları (customer-summary.ts) ---

/** Özet fonksiyonlarının döndürdüğü durum ve bilgi kartı; sync BaseProps'un kalanını ekler. */
export type SummaryBase = Pick<BaseProps, 'state' | 'card'>;

export type KorumaWarranty = {
  name: string;
  /** Uzun süre ("2 yıl 4 ay"), `customer` çevirisiyle. */
  leftLabel: string;
  /** Kilit boyu kısa süre ("2y 4a"). */
  leftShortLabel: string;
  /** Çubuk genişliği, 0-100. */
  percent: number;
  /** `percent_left < 15`: çubuk amber. */
  warn: boolean;
};

export type KorumaVehicle = {
  id: string;
  /** Araç sayfası bağlantısı. */
  url: string;
  /** Marka + model; kesme widget'ta yapılır. */
  title: string;
  plate: string;
  plateCountry: string;
  heroUri: string | null;
  brandLogoUri: string | null;
  brandInitial: string;
  activeCountLabel: string;
  lastServiceLabel: string;
  /** Artan `days_left`, en çok üç. */
  warranties: KorumaWarranty[];
  /** Aktif garanti yoksa gösterilen kart. */
  emptyCard: WidgetCard | null;
};

export type KorumaData = {
  eyebrow: string;
  leftCaption: string;
  /** İlk eleman otomatik seçimdir. */
  vehicles: KorumaVehicle[];
};
export type KorumaSummary = SummaryBase & KorumaData;
export type KorumaProps = BaseProps & KorumaData;

export type KartVehicle = {
  id: string;
  url: string;
  publicCode: string;
  productName: string;
  untilLabel: string;
  qrUri: string | null;
  heroUri: string | null;
  title: string;
  plate: string;
};

export type KartData = { title: string; vehicles: KartVehicle[] };
export type KartSummary = SummaryBase & KartData;
export type KartProps = BaseProps & KartData;

export type HizmetStep = { label: string; phase: 'done' | 'current' | 'todo' };

export type HizmetService = {
  uuid: string;
  url: string;
  serviceNo: string;
  dealerName: string;
  /** Geçerli adımın etiketi; tamamlandıysa "Tamamlandı". */
  statusLabel: string;
  done: boolean;
  steps: HizmetStep[];
};

export type HizmetVehicle = {
  id: string;
  url: string;
  title: string;
  plate: string;
  plateCountry: string;
  heroUri: string | null;
  brandLogoUri: string | null;
  brandInitial: string;
  /** `null`: gösterilecek hizmet yok, `emptyCard` çizilir. */
  service: HizmetService | null;
  emptyCard: WidgetCard | null;
};

export type HizmetData = { eyebrow: string; vehicles: HizmetVehicle[] };
export type HizmetSummary = SummaryBase & HizmetData;
export type HizmetProps = BaseProps & HizmetData;
