// Eşitleme kapısı: kısıt, sıra ve çıkış koruması. Saf dosya: saat ve depolama parametre gelir.
import { SYNC_MIN_INTERVAL_MS } from './config.ts';

/** `widgets.sync.v1` içeriği: yalnız zaman ve sahip, müşteri verisi yok. */
export type SyncRecord = { generatedAt: number; owner: string };

export type SyncTicket = { id: number; generation: number; owner: string; ownerChanged: boolean };

export function createSyncGate(minIntervalMs = SYNC_MIN_INTERVAL_MS) {
  let generation = 0;
  let latest = 0;
  let running = false;
  /** Diskteki kaydın sahibi ve zamanı (`restore`, `finish`). */
  let owner: string | null = null;
  /** Son başlayan çalıştırmanın sahibi: kısıt, kaydın bu sahibe ait olup olmadığına bakar. */
  let current: string | null = null;
  /** Widget'larda verisi tamamlanmış temizlikle ya da başarılı eşitlemeyle yerleşmiş sahip. */
  let settled: string | null = null;
  let generatedAt: number | null = null;
  /** Son deneme (başarısız olsa da): kısıt buna göre işler, kalıcı hata her öne gelişte yeniden çekmesin. */
  let attemptedAt: number | null = null;

  return {
    generation: () => generation,

    /** En son başlayan biletin kimliği; çıkış bunu değiştirmez. */
    latest: () => latest,

    /** Diskteki son kayıt; uygulama açılışında bir kez. */
    restore(saved: SyncRecord | null) {
      owner = saved?.owner ?? null;
      current = owner;
      settled = owner;
      generatedAt = saved?.generatedAt ?? null;
      attemptedAt = generatedAt;
    },

    /**
     * Zorlanmayan çağrı, süren eşitlemeyi ve 15 dakikalık kısıtı bekler; geriye giden saat kısıtı kilitlemez.
     * Geçerli sahip için kayıtlı bir eşitleme yoksa (ilk eşitleme, sahip değişimi) kısıt işlemez:
     * başarısız deneme sonraki öne gelişte yeniden denenir, widget giriş kartında takılı kalmaz.
     */
    shouldRun(now: number, force: boolean) {
      if (force) return true;
      if (running) return false;
      const hasRecord = generatedAt !== null && owner !== null && owner === current;
      if (!hasRecord) return true;
      return attemptedAt === null || now < attemptedAt || now - attemptedAt >= minIntervalMs;
    },

    /**
     * Yeni çalıştırma öncekileri geçersiz kılar. `ownerChanged`: başka bir sahibin verisi temizlenmeli;
     * temizlik `wiped` ile tamamlanana ya da başarılı bitişe kadar her bilet bunu yeniden bildirir.
     */
    begin(next: string, now?: number): SyncTicket {
      const ownerChanged = settled !== null && settled !== next;
      current = next;
      running = true;
      if (now !== undefined) attemptedAt = now;
      return { id: ++latest, generation, owner: next, ownerChanged };
    },

    /** Sahip değişimi temizliği tamamlandı; yazma hakkı yoksa (yarım kaldıysa) bir sonraki bilet yeniden yapar. */
    wiped(ticket: SyncTicket) {
      if (this.canWrite(ticket)) settled = ticket.owner;
    },

    /** Çıkıştan sonra görseller yalnız o çıkıştan beri yeni eşitleme başlamadıysa silinir; yenisinin dosyalarına dokunulmaz. */
    shouldPurge(ticket: SyncTicket) {
      return ticket.generation !== generation && ticket.id === latest;
    },

    /** Çıkış ya da daha yeni bir çalıştırma geçtiyse yazma. */
    canWrite(ticket: SyncTicket) {
      return ticket.generation === generation && ticket.id === latest;
    },

    /** Başarılı bitiş: zamanı kaydeder. Yazma hakkı yoksa false. */
    finish(ticket: SyncTicket, now: number) {
      if (!this.canWrite(ticket)) return false;
      running = false;
      owner = ticket.owner;
      settled = ticket.owner;
      generatedAt = now;
      attemptedAt = now;
      return true;
    },

    /** Tam başarıya varmayan çalıştırma: kayıt zamanı yazılmaz, deneme zamanı (`begin`) kısıtta kalır. */
    abort(ticket: SyncTicket) {
      if (this.canWrite(ticket)) running = false;
    },

    /** Çıkış: nesil artar, uçuştaki çalıştırmalar yazamaz, kısıt ve sahip sıfırlanır. */
    invalidate() {
      generation++;
      running = false;
      owner = null;
      current = null;
      settled = null;
      generatedAt = null;
      attemptedAt = null;
    },

    snapshot: () => ({ generatedAt, owner }),
  };
}

export type SyncGate = ReturnType<typeof createSyncGate>;
