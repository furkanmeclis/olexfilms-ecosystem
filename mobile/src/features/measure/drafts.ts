import { create } from 'zustand';

import { readJSON, writeJSON } from '@/core/storage';

import type { PointReading } from './guided';

/** Raw probe data of one gauge reading (TEC-939), kept so a draft saves the same payload later. */
export type DraftGaugeRead = {
  part_type: string;
  value_um: number;
  frequency_hz: number;
  channel: 'fe' | 'al';
  material: 'fe' | 'al';
  trigger: 'steady' | 'button';
  fe_offset_hz: number | null;
  al_offset_hz: number | null;
  at: string;
};

/**
 * A measurement not sent yet, kept on this phone only (user decision
 * 11.11): plate is required, VIN and body type optional; sending
 * (`POST /v1/mobile/measurements`) needs the VIN.
 */
export type MeasurementDraft = {
  /** Also the Idempotency-Key / client_measurement_id when it is sent. */
  id: string;
  plate: string;
  vin: string;
  bodyType: string | null;
  /** Part key → readings as shown (local number format). */
  values: Record<string, string>;
  substrates: Record<string, 'Fe' | 'Al'>;
  gaugeDevice: { serial: string; model?: string } | null;
  gaugeReads: DraftGaugeRead[];
  /** Guided mode (body type picked): readings per point (`PART#position`), the current point and the order taken. */
  points?: Record<string, PointReading>;
  cursor?: number;
  history?: string[];
  updatedAt: string;
};

type DraftState = {
  userKey: string | null;
  drafts: MeasurementDraft[];
  load: (userKey: string | null) => void;
  save: (draft: MeasurementDraft) => void;
  remove: (id: string) => void;
};

const keyOf = (userKey: string) => `measure.drafts.${userKey}`;

function persist(userKey: string | null, drafts: MeasurementDraft[]) {
  if (userKey) writeJSON(keyOf(userKey), drafts);
}

/** Drafts of the signed-in user, newest first. */
export const useDrafts = create<DraftState>((set, get) => ({
  userKey: null,
  drafts: [],
  load: (userKey) => {
    if (get().userKey === userKey) return;
    set({ userKey, drafts: userKey ? (readJSON<MeasurementDraft[]>(keyOf(userKey)) ?? []) : [] });
  },
  save: (draft) => {
    const next = [{ ...draft, updatedAt: new Date().toISOString() }, ...get().drafts.filter((d) => d.id !== draft.id)];
    set({ drafts: next });
    persist(get().userKey, next);
  },
  remove: (id) => {
    const next = get().drafts.filter((d) => d.id !== id);
    set({ drafts: next });
    persist(get().userKey, next);
  },
}));

/** Number of readings in a draft. */
export function draftReadingCount(d: MeasurementDraft): number {
  const free = Object.values(d.values).reduce((n, v) => n + v.trim().split(/\s+/u).filter(Boolean).length, 0);
  return free + Object.keys(d.points ?? {}).length;
}
