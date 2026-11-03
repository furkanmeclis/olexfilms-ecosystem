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
  plateFg: string;
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
  /** "2 yıl 4 ay kaldı". */
  leftLongLabel: string;
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
// --- Bayi widget'ları (dealer-summary.ts) ---

export type PanomRow = {
  uuid: string;
  url: string;
  carTitle: string;
  plate: string;
  plateCountry: string;
  /** İlk kategori + bekleme süresi. */
  meta: string;
  /** SF Symbol adı. */
  statusIcon: string;
  statusTone: 'neutral' | 'progress' | 'done' | 'problem';
  logoUri: string | null;
  initial: string;
};

export type PanomLongest = { uuid: string; url: string; carTitle: string; plate: string; waitLabel: string };

export type PanomData = {
  title: string;
  /** Sayılar dilin rakamlarıyla biçimlenmiş. */
  next: string;
  active: string;
  done: string;
  rows: PanomRow[];
  longest: PanomLongest | null;
};
export type PanomSummary = SummaryBase & PanomData;
export type PanomProps = BaseProps & PanomData;

export type QuickAction = { id: 'scan' | 'new_service' | 'measure' | 'customers' | 'services'; label: string; url: string };

export type QuickData = {
  title: string;
  actions: QuickAction[];
  /** "1.284 kayıt"; yalnız müşteriler düğmesi varsa ve sayı biliniyorsa. */
  customersCount: string | null;
};
export type QuickSummary = SummaryBase & QuickData;
export type QuickProps = BaseProps & QuickData;

export type KpiCard = { key: string; label: string; value: string };

export type KpiBar = { label: string; height: number; current: boolean };

export type KpiData = {
  title: string;
  weeklyTitle: string;
  cards: KpiCard[];
  /** Küçük boy ana değeri. */
  primary: KpiCard | null;
  /** "Açılan 42 · Devam 9"; iki kart da varsa. */
  summary: string | null;
  /** Son beş hafta, pt cinsinden yükseklik. */
  bars: KpiBar[];
};
export type KpiSummary = SummaryBase & KpiData;
export type KpiProps = BaseProps & KpiData;

export type PendingRow = { id: 'ready' | 'claim' | 'expiring'; label: string; detail: string; count: string; url: string };

export type PendingData = { title: string; rows: PendingRow[] };
export type PendingSummary = SummaryBase & PendingData;
export type PendingProps = BaseProps & PendingData;

export type RevenueData = {
  title: string;
  /** "Kasım geliri" */
  monthTitle: string;
  currency: string;
  incomeLabel: string;
  netLabel: string;
  expenseLabel: string;
  maskedLabel: string;
  netTitle: string;
  expenseTitle: string;
  hiddenTitle: string;
  tapToReveal: string;
  /** Varsayılan gizli. */
  revealed: boolean;
};
export type RevenueSummary = SummaryBase & RevenueData;
export type RevenueProps = BaseProps & RevenueData;
