import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';

import { noContent } from './api';

type S = components['schemas'];
export type WarehouseLocation = S['WarehouseLocation'];
export type LocationType = S['WarehouseLocationType'];
export type GenerateLevel = S['WarehouseGenerateLevel'];

/** Setup screens (panel routes through the mobile mirror). */
export const sk = {
  warehouses: () => ['warehouse', 'setup', 'warehouses'] as const,
  warehouse: (uuid: string) => ['warehouse', 'setup', 'warehouse', uuid] as const,
  rooms: (warehouse: string) => ['warehouse', 'setup', 'rooms', warehouse] as const,
  room: (uuid: string) => ['warehouse', 'setup', 'room', uuid] as const,
  locations: (room: string) => ['warehouse', 'setup', 'locations', room] as const,
};

/** Every warehouse, inactive ones too (the picker only lists active ones). */
export function useAllWarehouses() {
  return useQuery({
    queryKey: sk.warehouses(),
    queryFn: () => unwrap(api.GET('/v1/warehouse/warehouses')),
    select: (d) => [...d.items].sort((a, b) => a.sort_order - b.sort_order || a.code.localeCompare(b.code)),
  });
}

export function useSetupWarehouse(uuid: string, enabled: boolean) {
  return useQuery({
    queryKey: sk.warehouse(uuid),
    enabled,
    queryFn: () => unwrap(api.GET('/v1/warehouse/warehouses/{uuid}', { params: { path: { uuid } } })),
  });
}

export function useSetupRooms(warehouse: string, enabled: boolean) {
  return useQuery({
    queryKey: sk.rooms(warehouse),
    enabled,
    queryFn: () => unwrap(api.GET('/v1/warehouse/warehouses/{uuid}/rooms', { params: { path: { uuid: warehouse } } })),
    select: (d) => [...d.items].sort((a, b) => a.sort_order - b.sort_order || a.code.localeCompare(b.code)),
  });
}

export function useSetupRoom(uuid: string) {
  return useQuery({
    queryKey: sk.room(uuid),
    queryFn: () => unwrap(api.GET('/v1/warehouse/rooms/{uuid}', { params: { path: { uuid } } })),
  });
}

export function useRoomLocations(room: string) {
  return useQuery({
    queryKey: sk.locations(room),
    queryFn: () => unwrap(api.GET('/v1/warehouse/rooms/{uuid}/locations', { params: { path: { uuid: room } } })),
    select: (d) => d.items,
  });
}

export const setupApi = {
  createWarehouse: (body: S['WarehouseInput']) => unwrap(api.POST('/v1/warehouse/warehouses', { body })),
  updateWarehouse: (uuid: string, body: S['WarehousePatch']) =>
    unwrap(api.PATCH('/v1/warehouse/warehouses/{uuid}', { params: { path: { uuid } }, body })),
  deleteWarehouse: (uuid: string) =>
    noContent(api.DELETE('/v1/warehouse/warehouses/{uuid}', { params: { path: { uuid } } })),
  reorderWarehouses: (uuids: string[]) => noContent(api.POST('/v1/warehouse/warehouses/reorder', { body: { uuids } })),

  createRoom: (warehouse: string, body: S['WarehouseRoomInput']) =>
    unwrap(api.POST('/v1/warehouse/warehouses/{uuid}/rooms', { params: { path: { uuid: warehouse } }, body })),
  updateRoom: (uuid: string, body: S['WarehouseRoomPatch']) =>
    unwrap(api.PATCH('/v1/warehouse/rooms/{uuid}', { params: { path: { uuid } }, body })),
  deleteRoom: (uuid: string) => noContent(api.DELETE('/v1/warehouse/rooms/{uuid}', { params: { path: { uuid } } })),
  reorderRooms: (uuids: string[]) => noContent(api.POST('/v1/warehouse/rooms/reorder', { body: { uuids } })),

  createLocation: (body: S['WarehouseLocationInput']) => unwrap(api.POST('/v1/warehouse/locations', { body })),
  updateLocation: (uuid: string, body: S['WarehouseLocationPatch']) =>
    unwrap(api.PATCH('/v1/warehouse/locations/{uuid}', { params: { path: { uuid } }, body })),
  deleteLocation: (uuid: string) =>
    noContent(api.DELETE('/v1/warehouse/locations/{uuid}', { params: { path: { uuid } } })),
  reorderLocations: (uuids: string[]) => noContent(api.POST('/v1/warehouse/locations/reorder', { body: { uuids } })),
  generate: (body: S['WarehouseGenerateInput']) => unwrap(api.POST('/v1/warehouse/locations/generate', { body })),
};

/** Child types a location may have (web rule): room root → aisle or shelf, aisle → shelf, shelf → bin. */
export function allowedChildTypes(parent: LocationType | null): LocationType[] {
  if (parent === null) return ['aisle', 'shelf'];
  if (parent === 'aisle') return ['shelf'];
  if (parent === 'shelf') return ['bin'];
  return [];
}

/** Codes: letters, digits and _, up to 32, stored upper-case. */
export const CODE_RE = /^[A-Z0-9_]{1,32}$/;
export const normalizeCode = (s: string) => s.trim().toUpperCase();

/** Moves one uuid up or down in its sibling list; returns the new order or null at the edge. */
export function moved(uuids: string[], uuid: string, dir: -1 | 1): string[] | null {
  const i = uuids.indexOf(uuid);
  const j = i + dir;
  if (i < 0 || j < 0 || j >= uuids.length) return null;
  const out = [...uuids];
  [out[i], out[j]] = [out[j], out[i]];
  return out;
}
