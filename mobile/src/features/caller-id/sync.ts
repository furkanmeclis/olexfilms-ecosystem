import { api, unwrap } from '@/core/api/client';
import { ApiError } from '@/core/api/errors';
import type { MobileMe } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';

import { buildPhoneBook } from './entries';
import { callerIdNative } from './native';
import { scheduleCallerIdTask } from './schedule';
import { type CallerIdError, currentTicket, EMPTY_LIST, newTicket, useCallerId } from './store';

/** A foreground return refreshes the list at most this often. */
export const SYNC_MIN_INTERVAL_MS = 15 * 60_000;

/** Staff who may read customers in their active organization. Customer and fleet accounts never. */
export function callerIdEligible(me: MobileMe | null): boolean {
  if (!me || me.session_kind === 'customer' || !me.active_organization_uuid) return false;
  return me.user.is_super_admin === true || (me.permissions ?? []).includes('customers.read');
}

/** The list on the device belongs to one user in one organization. */
export function callerIdOwner(me: MobileMe | null): string | null {
  return me?.active_organization_uuid ? `${me.user.uuid}:${me.active_organization_uuid}` : null;
}

function classify(e: unknown): CallerIdError {
  if (e instanceof ApiError) {
    if (e.status === 0) return 'network';
    if (e.status === 403) return 'forbidden';
  }
  return 'failed';
}

async function wipe(): Promise<void> {
  newTicket();
  useCallerId.getState().patch(EMPTY_LIST);
  await callerIdNative.clear();
}

async function run(opts: { force?: boolean; full?: boolean }): Promise<void> {
  if (!callerIdNative.available) return;
  const store = useCallerId.getState();
  const me = useSession.getState().me;
  const owner = callerIdOwner(me);
  const on = !!me && !!owner && store.enabledBy === me.user.uuid && callerIdEligible(me);
  if (!on) {
    // Switched off, another user, or the role lost customers.read: no list stays behind.
    if (store.owner) await wipe();
    return;
  }
  // Another organization's customers must not answer while the new list loads.
  if (store.owner && store.owner !== owner) await wipe();

  const { owner: held, syncedAt, version } = useCallerId.getState();
  if (!opts.force && held === owner && syncedAt && Date.now() - syncedAt < SYNC_MIN_INTERVAL_MS) return;

  const ticket = currentTicket();
  useCallerId.setState({ busy: true });
  try {
    const known = !opts.full && held === owner && version ? version : undefined;
    const snap = await unwrap(api.GET('/v1/customers/caller-id', { params: { query: { known_version: known } } }));
    if (ticket !== currentTicket()) return;
    if (!snap.unchanged) {
      await callerIdNative.setEntries(buildPhoneBook(snap.entries).tsv);
      if (ticket !== currentTicket()) {
        // Switched off or signed out while the list was being written.
        await callerIdNative.clear();
        return;
      }
    }
    await callerIdNative.setEnabled(true);
    useCallerId.getState().patch({
      owner,
      version: snap.version,
      count: snap.count,
      truncated: snap.truncated,
      syncedAt: Date.now(),
    });
    useCallerId.setState({ error: null });
  } catch (e) {
    if (ticket === currentTicket()) useCallerId.setState({ error: classify(e) });
  } finally {
    useCallerId.setState({ busy: false });
  }
}

let running: Promise<void> | null = null;

/**
 * Brings the list on the device in line with the server and the switch.
 * `force` skips the 15 minute limit; `full` downloads the list even when the
 * version still matches. Never rejects: the outcome is in the store.
 */
export function syncCallerId(opts: { force?: boolean; full?: boolean } = {}): Promise<void> {
  // A plain refresh joins the run in flight; a forced one (switch, owner
  // change) runs after it, so it always sees the latest session.
  if (running && !opts.force) return running;
  const started = running ? running.then(() => run(opts)) : run(opts);
  const mine: Promise<void> = started
    .catch(() => undefined)
    .finally(() => {
      if (running === mine) running = null;
    });
  running = mine;
  return mine;
}

/** The in-app switch, on: downloads the whole list and starts the periodic sync. */
export async function enableCallerId(): Promise<void> {
  const me = useSession.getState().me;
  if (!me || !callerIdEligible(me) || !callerIdNative.available) return;
  useCallerId.getState().patch({ enabledBy: me.user.uuid });
  await syncCallerId({ force: true, full: true });
  void scheduleCallerIdTask(true);
  void useCallerId.getState().refreshSystem();
}

/** The in-app switch, off: the list leaves the device and nothing syncs any more. */
export async function disableCallerId(): Promise<void> {
  useCallerId.getState().patch({ enabledBy: null });
  useCallerId.setState({ error: null });
  try {
    await callerIdNative.setEnabled(false);
    await wipe();
  } catch {
    // The next sync sees the switch off and wipes again.
  }
  void scheduleCallerIdTask(false);
}
