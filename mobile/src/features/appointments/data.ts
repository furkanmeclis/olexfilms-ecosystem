import { api, unwrap } from '@/core/api/client';

import type { Appointment } from './lib';

const PAGE = 100;
const MAX_PAGES = 5;

/** Every appointment starting in [from, to), paged by the list contract. */
export async function loadRange(from: Date, to: Date) {
  const items: Appointment[] = [];
  for (let page = 0; page < MAX_PAGES; page++) {
    const res = await unwrap(
      api.GET('/v1/appointments', {
        params: { query: { from: from.toISOString(), to: to.toISOString(), limit: PAGE, offset: page * PAGE } },
      }),
    );
    items.push(...res.items);
    if (items.length >= res.total || res.items.length < PAGE) break;
  }
  return items.sort((a, b) => a.starts_at.localeCompare(b.starts_at));
}

const DAY_MS = 86_400_000;

/**
 * One booking by uuid. The API has no single-get, so the cached week lists are
 * searched first, then a wide window around today (deep links from a push).
 */
export async function findAppointment(uuid: string, cached: Appointment[]) {
  const hit = cached.find((a) => a.uuid === uuid);
  if (hit) return hit;
  const now = Date.now();
  const all = await loadRange(new Date(now - 90 * DAY_MS), new Date(now + 365 * DAY_MS));
  return all.find((a) => a.uuid === uuid) ?? null;
}
