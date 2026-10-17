import { useInfiniteQuery } from '@tanstack/react-query';
import { type Href, useSegments } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { Share } from 'react-native';

import { isApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { useSession } from '@/core/auth/session';
import { PAGE_SIZE, publicWarrantyUrl, WEB_ORIGIN } from '@/features/services/lib';
import type { StatusTone } from '@/ui/status-pill';

type S = components['schemas'];
export type PortalVehicle = S['PortalVehicle'];
export type PortalVehicleDetail = S['PortalVehicleDetail'];
export type PortalServiceItem = S['PortalServiceListItem'];
export type PortalService = S['PortalService'];
export type PortalActiveWarranty = S['PortalActiveWarranty'];
export type PortalAppointment = S['PortalAppointment'];
export type AppointmentStatus = S['AppointmentStatus'];
export type FleetOverview = S['FleetPortalOverview'];
export type NotificationPreferences = S['NotificationPreferences'];

export { PAGE_SIZE };

/** Statuses the portal labels itself (the backend label is the fallback). */
export const PORTAL_SERVICE_STATUSES = ['draft', 'pending', 'processing', 'ready', 'completed', 'cancelled'] as const;

export function portalServiceTone(s: string | undefined): StatusTone {
  if (s === 'completed') return 'done';
  if (s === 'cancelled') return 'problem';
  if (s === 'draft' || !s) return 'neutral';
  return 'progress';
}

export function appointmentTone(s: AppointmentStatus | undefined): StatusTone {
  if (s === 'confirmed' || s === 'arrived') return 'done';
  if (s === 'cancelled' || s === 'no_show') return 'problem';
  return 'progress';
}

/** Appointments the customer may still cancel (the backend closes the window 2 h before). */
export function canCancelAppointment(a: PortalAppointment) {
  return (a.status === 'scheduled' || a.status === 'confirmed') && new Date(a.starts_at).getTime() > Date.now();
}

/** 403 / 404 on an optional section: hide it instead of showing an error. */
export function isHidden(e: unknown) {
  return isApiError(e) && (e.status === 403 || e.status === 404);
}

/** Do not retry answers that will not change (forbidden, not found, bad request). */
export function retryPortal(count: number, e: unknown) {
  if (isApiError(e) && e.status >= 400 && e.status < 500) return false;
  return count < 2;
}

/** Fleet portal user (web: `isPortalReadOnly`): roles include fleet, not customer. */
export function useFleetRoles() {
  const roles = useSession((s) => s.me?.roles);
  const fleet = !!roles?.includes('fleet');
  return { fleet, readOnly: fleet && !roles?.includes('customer') };
}

/** Public dealer finder of the web (no portal session needed). */
export const DEALER_FINDER_URL = `${WEB_ORIGIN}/portal/dealers`;

export async function openDealerFinder() {
  await WebBrowser.openBrowserAsync(DEALER_FINDER_URL);
}

export async function openPublicWarranty(code: string) {
  await WebBrowser.openBrowserAsync(publicWarrantyUrl(code));
}

export function sharePublicWarranty(code: string, message: string) {
  const url = publicWarrantyUrl(code);
  void Share.share({ message: `${message}\n${url}`, url });
}

/** WhatsApp chat link of an E.164 number (`+90…` → `https://wa.me/90…`). */
export function whatsappUrl(phone: string) {
  return `https://wa.me/${phone.replace(/\D/g, '')}`;
}

export function daysLeft(endIso: string | null | undefined) {
  if (!endIso) return null;
  return Math.ceil((new Date(endIso).getTime() - Date.now()) / 86_400_000);
}

export function nameOf(ref: { name?: string | null } | null | undefined) {
  return ref?.name ?? '';
}

type Paged<T> = { items: T[]; total: number; limit?: number; offset?: number };

/** Offset-paged portal list as an infinite query (works for pages with or without limit/offset). */
export function usePortalPages<T>(key: readonly unknown[], fetchPage: (offset: number) => Promise<Paged<T>>) {
  const q = useInfiniteQuery({
    queryKey: key,
    initialPageParam: 0,
    retry: retryPortal,
    queryFn: async ({ pageParam }) => {
      const p = await fetchPage(pageParam);
      return { items: p?.items ?? [], total: p?.total ?? 0, offset: pageParam };
    },
    getNextPageParam: (last) => {
      const next = last.offset + last.items.length;
      return last.items.length > 0 && next < last.total ? next : undefined;
    },
  });
  const items = q.data?.pages.flatMap((p) => p.items) ?? [];
  const total = q.data?.pages[0]?.total;
  return { ...q, items, total };
}

type Kind = 'vehicle' | 'service' | 'warranty';

/**
 * Detail links that stay inside the current tab's stack. The home tab owns
 * `vehicle/`, `service/`, `warranty/`; the warranties tab owns `[uuid]`
 * (warranty) plus `service/` and `vehicle/`.
 */
export function useCustomerLinks() {
  const seg = useSegments() as string[];
  const tab = seg[2];
  const to = (kind: Kind, id: string): Href => {
    const uid = encodeURIComponent(id);
    if (tab === 'warranties') {
      return (kind === 'warranty' ? `/musteri/warranties/${uid}` : `/musteri/warranties/${kind}/${uid}`) as Href;
    }
    return `/musteri/${kind}/${uid}` as Href;
  };
  return {
    vehicle: (id: string) => to('vehicle', id),
    service: (id: string) => to('service', id),
    warranty: (id: string) => to('warranty', id),
  };
}
