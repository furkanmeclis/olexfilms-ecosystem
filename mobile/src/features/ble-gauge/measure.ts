/**
 * Frame → reading (pure, alias-free): the substrate decision and the channel
 * rule of the NexPTG app (docs/smartptg/REVERSE_ENGINEERING.md §4, §5.4).
 *
 * In mode 3 the gauge alternates the Fe and the Al channel in groups of ~8
 * frames. The substrate J comes from the last Fe channel frequency against
 * the curve's K threshold (or, without one, from this frame against K / L).
 * Only a frame of J's own channel is computed; a frame of the other channel
 * keeps the previous value (`held`), it does not mean the probe lifted.
 * A value without microns (air / off-scale) forgets the last Fe frequency.
 */
import {
  applyDeviceRules,
  type CurveOffsets,
  frequencyToMicrons,
  type GaugeCurve,
  type GaugeReading,
  type Material,
  materialOf,
} from './curve.ts';
import type { GaugeFrame } from './protocol.ts';

export type MeasureState = {
  /** Frequency of the last Fe channel frame of the contact (NexPTGService.Y). */
  feFrequency: number | null;
  /** Substrate and value of the last computed frame (NexPTGService.f14439q). */
  material: Material | null;
  um: number | null;
};

export const initialMeasure: MeasureState = { feFrequency: null, material: null, um: null };

/**
 * One frame. Returns a null reading for a frame of an unknown type. Without
 * the serial's curve the reading has no microns and no substrate.
 */
export function stepMeasure(
  state: MeasureState,
  frame: GaugeFrame,
  curve: GaugeCurve | null,
  offsets: CurveOffsets | null,
  at: number,
): { state: MeasureState; reading: GaugeReading | null } {
  const channel = frame.channel;
  if (!channel) return { state, reading: null };
  const base = {
    channel,
    frequency: frame.frequency,
    serial: frame.serial,
    batteryVoltage: frame.batteryVoltage,
    at,
  };
  if (!curve || curve.serial !== frame.serial) {
    return { state: initialMeasure, reading: { ...base, um: null, material: null, held: false, button: false } };
  }

  const feFrequency = channel === 'fe' ? frame.frequency : state.feFrequency;
  const material = feFrequency != null ? materialOf('fe', feFrequency, curve) : materialOf(channel, frame.frequency, curve);

  if (material !== channel) {
    // The other channel: keep the previous value of this substrate.
    const um = state.material === material ? state.um : null;
    return {
      state: { ...state, feFrequency },
      reading: { ...base, um, material, held: true, button: false },
    };
  }

  const raw = frequencyToMicrons(material, frame.frequency, curve, offsets);
  const um = applyDeviceRules(raw, material, curve.deviceType);
  return {
    state: { feFrequency: um == null ? null : feFrequency, material, um },
    reading: { ...base, um, material, held: false, button: frame.button },
  };
}
