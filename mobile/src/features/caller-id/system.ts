/**
 * What the system still needs from the user before calls are identified
 * (TEC-1208). Pure, so it is tested without a device.
 */
export type CallerIdStep = 'ios_extension' | 'android_role' | 'android_overlay';

export type CallerIdSystem = {
  /** The device and build can do this at all. */
  supported: boolean;
  /** What the user has to allow in the system, in order. */
  steps: { id: CallerIdStep; done: boolean }[];
  /** Every step done: calls are identified. */
  ready: boolean;
};

export type NativeStatus = {
  supported: boolean;
  extension?: 'enabled' | 'disabled' | 'unknown';
  role?: boolean;
  overlay?: boolean;
};

export const UNSUPPORTED: CallerIdSystem = { supported: false, steps: [], ready: false };

export function toSystem(platform: string, s: NativeStatus): CallerIdSystem {
  if (!s.supported) return UNSUPPORTED;
  const steps: CallerIdSystem['steps'] =
    platform === 'ios'
      ? [{ id: 'ios_extension', done: s.extension === 'enabled' }]
      : [
          { id: 'android_role', done: s.role === true },
          { id: 'android_overlay', done: s.overlay === true },
        ];
  return { supported: true, steps, ready: steps.every((x) => x.done) };
}
