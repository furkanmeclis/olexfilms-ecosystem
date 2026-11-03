// Eşitleme kapısı: kısıt, sıra ve çıkış koruması. Saf dosya: saat ve depolama parametre gelir.
import { SYNC_MIN_INTERVAL_MS } from './config.ts';

/** `widgets.sync.v1` içeriği: yalnız zaman ve sahip, müşteri verisi yok. */
export type SyncRecord = { generatedAt: number; owner: string };

export type SyncTicket = { id: number; generation: number; owner: string; ownerChanged: boolean };

export function createSyncGate(minIntervalMs = SYNC_MIN_INTERVAL_MS) {
  let generation = 0;
  let latest = 0;
  let running = false;
  let owner: string | null = null;
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
      generatedAt = saved?.generatedAt ?? null;
      attemptedAt = generatedAt;
    },

    /** Zorlanmayan çağrı, süren eşitlemeyi ve 15 dakikalık kısıtı bekler; geriye giden saat kısıtı kilitlemez. */
    shouldRun(now: number, force: boolean) {
      if (force) return true;
      if (running) return false;
      return attemptedAt === null || now < attemptedAt || now - attemptedAt >= minIntervalMs;
    },

    /** Yeni çalıştırma öncekileri geçersiz kılar. `ownerChanged`: kayıtlı başka bir sahibin verisi temizlenmeli. */
    begin(next: string, now?: number): SyncTicket {
      const ownerChanged = owner !== null && owner !== next;
      owner = next;
      running = true;
      if (now !== undefined) attemptedAt = now;
      return { id: ++latest, generation, owner: next, ownerChanged };
    },

    /** Çıkış ya da daha yeni bir çalıştırma geçtiyse yazma. */
    canWrite(ticket: SyncTicket) {
      return ticket.generation === generation && ticket.id === latest;
    },

    /** Başarılı bitiş: zamanı kaydeder. Yazma hakkı yoksa false. */
    finish(ticket: SyncTicket, now: number) {
      if (!this.canWrite(ticket)) return false;
      running = false;
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
      generatedAt = null;
      attemptedAt = null;
    },

    snapshot: () => ({ generatedAt, owner }),
  };
}

export type SyncGate = ReturnType<typeof createSyncGate>;
