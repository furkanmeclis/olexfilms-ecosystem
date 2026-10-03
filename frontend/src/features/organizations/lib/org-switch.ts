import { useSyncExternalStore } from "react";

/**
 * TEC-227: one-way organization switch.
 *
 * While the header switcher moves the session to a new organization, the
 * previous organization's `TenantOrganizationContext` stays mounted until
 * the navigation commits. Seeing a session that no longer matches its slug,
 * it would scope the session back to the old organization. The switcher
 * marks the target here; every context whose slug differs pauses its sync
 * until the target's context takes over (or the safety timeout expires,
 * e.g. when the navigation never lands).
 */

export const ORG_SWITCH_TIMEOUT_MS = 15_000;

let target: string | null = null;
let timer: ReturnType<typeof setTimeout> | null = null;
const listeners = new Set<() => void>();

function emit() {
  for (const listener of listeners) listener();
}

function set(next: string | null) {
  if (timer) {
    clearTimeout(timer);
    timer = null;
  }
  target = next;
  if (next !== null) {
    timer = setTimeout(() => set(null), ORG_SWITCH_TIMEOUT_MS);
  }
  emit();
}

/** Marks a switch to `slug` as in progress. */
export function beginOrgSwitch(slug: string) {
  set(slug);
}

/** Ends the switch to `slug`; a no-op when another switch is pending. */
export function endOrgSwitch(slug: string) {
  if (target === slug) set(null);
}

export function getOrgSwitchTarget() {
  return target;
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** The slug a switch is heading to, or null when no switch is pending. */
export function useOrgSwitchTarget() {
  return useSyncExternalStore(subscribe, getOrgSwitchTarget, () => null);
}
