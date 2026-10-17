import { useInfiniteQuery } from '@tanstack/react-query';
import { type Href, useSegments } from 'expo-router';
import { useEffect, useState } from 'react';

import type { components } from '@/core/api/schema';
import { API_URL } from '@/core/config';
import type { StatusTone } from '@/ui/status-pill';

type S = components['schemas'];
export type Service = S['Service'];
export type ServiceStatus = S['ServiceStatus'];
export type ServiceItem = S['ServiceItem'];
export type ServiceStockUnit = S['ServiceStockUnit'];
export type ServiceWarranty = S['ServiceWarranty'];
export type CustomerSummary = S['CustomerSummary'];
export type CustomerDetail = S['CustomerDetail'];
export type Vehicle = S['Vehicle'];
export type Warranty = S['Warranty'];
export type WarrantyStatus = Warranty['status'];

export type Page<T> = { items: T[]; total: number; limit: number; offset: number };

export const PAGE_SIZE = 20;

export const SERVICE_STATUSES: ServiceStatus[] = ['draft', 'pending', 'processing', 'ready', 'completed', 'cancelled'];
export const WARRANTY_STATUSES: WarrantyStatus[] = ['active', 'expired', 'void'];

export function serviceTone(s: ServiceStatus): StatusTone {
  if (s === 'completed') return 'done';
  if (s === 'cancelled') return 'problem';
  if (s === 'draft') return 'neutral';
  return 'progress';
}

export function warrantyTone(s: WarrantyStatus): StatusTone {
  if (s === 'active') return 'done';
  if (s === 'void') return 'problem';
  return 'neutral';
}

/** Web origin of the BFF (`https://host/api` → `https://host`). */
export const WEB_ORIGIN = API_URL.replace(/\/api$/, '');

/** Public warranty page, the one printed on the certificate. */
export function publicWarrantyUrl(code: string) {
  return `${WEB_ORIGIN}/warranty/${encodeURIComponent(code)}`;
}

export function formatDate(iso: string | null | undefined, locale: string) {
  if (!iso) return '';
  return new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(iso));
}

export function formatDateTime(iso: string | null | undefined, locale: string) {
  if (!iso) return '';
  return new Intl.DateTimeFormat(locale, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(iso));
}

export function formatNumber(n: number, locale: string) {
  return new Intl.NumberFormat(locale).format(n);
}

export function fullName(p: { name: string; surname?: string | null } | null | undefined) {
  if (!p) return '';
  return [p.name, p.surname].filter(Boolean).join(' ');
}

export function vehicleLabel(brand?: string | null, model?: string | null, year?: number | null) {
  return [brand, model, year ? String(year) : null].filter(Boolean).join(' ');
}

/** Waits for typing to settle before a search hits the API. */
export function useDebounced<T>(value: T, ms = 300) {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

/** Offset-paged list (`{ items, total, limit, offset }`) as an infinite query. */
export function usePagedList<T>(key: readonly unknown[], fetchPage: (offset: number) => Promise<Page<T>>) {
  const q = useInfiniteQuery({
    queryKey: key,
    initialPageParam: 0,
    queryFn: ({ pageParam }) => fetchPage(pageParam),
    getNextPageParam: (last) => {
      const next = last.offset + last.items.length;
      return last.items.length > 0 && next < last.total ? next : undefined;
    },
  });
  const items = q.data?.pages.flatMap((p) => p.items) ?? [];
  const total = q.data?.pages[0]?.total;
  return { ...q, items, total };
}

type EntityKind = 'services' | 'customers' | 'warranties';

/**
 * Detail links that stay inside the current tab's stack. Every feature
 * folder (`<space>/services`, `<space>/menu/customers`, `<space>/menu/warranties`)
 * carries detail routes for the other two kinds, so a pushed screen never
 * jumps to another tab. Home links open the services tab.
 */
export function useLinks() {
  const seg = useSegments() as string[];
  const space = seg[1] ?? 'bayi';
  const tab = seg[2];
  let base = `/${space}/services`;
  let self: EntityKind = 'services';
  if (tab === 'menu' && (seg[3] === 'customers' || seg[3] === 'warranties')) {
    base = `/${space}/menu/${seg[3]}`;
    self = seg[3];
  }
  const to = (kind: EntityKind, id: string) => (kind === self ? `${base}/${id}` : `${base}/${kind}/${id}`) as Href;
  return {
    service: (id: string) => to('services', id),
    addItem: (id: string) => `${to('services', id) as string}/add-item` as Href,
    customer: (id: string) => to('customers', id),
    warranty: (id: string) => to('warranties', id),
    newService: `/${space}/services/new` as Href,
  };
}
