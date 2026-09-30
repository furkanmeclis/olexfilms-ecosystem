"use client";

import { useCallback, useSyncExternalStore } from "react";

const STORAGE_PREFIX = "app.nav.groups.v1";
const NAV_GROUPS_EVENT = "app:nav-groups-change";

function storageKey(catalogId: string) {
  return `${STORAGE_PREFIX}:${catalogId}`;
}

function readGroupMap(catalogId: string): Record<string, boolean> {
  if (typeof window === "undefined") return {};
  try {
    const raw = window.localStorage.getItem(storageKey(catalogId));
    if (!raw) return {};
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== "object") return {};
    return parsed as Record<string, boolean>;
  } catch {
    return {};
  }
}

function writeGroupOpen(catalogId: string, groupId: string, open: boolean) {
  if (typeof window === "undefined") return;
  const next = { ...readGroupMap(catalogId), [groupId]: open };
  window.localStorage.setItem(storageKey(catalogId), JSON.stringify(next));
  window.dispatchEvent(new Event(NAV_GROUPS_EVENT));
}

function subscribe(onStoreChange: () => void) {
  const handleStorage = (event: StorageEvent) => {
    if (event.key?.startsWith(STORAGE_PREFIX) || event.key === null) {
      onStoreChange();
    }
  };
  window.addEventListener("storage", handleStorage);
  window.addEventListener(NAV_GROUPS_EVENT, onStoreChange);
  return () => {
    window.removeEventListener("storage", handleStorage);
    window.removeEventListener(NAV_GROUPS_EVENT, onStoreChange);
  };
}

export function useNavGroupOpen(
  catalogId: string,
  groupId: string,
  defaultOpen: boolean,
) {
  const open = useSyncExternalStore(
    subscribe,
    () => {
      const stored = readGroupMap(catalogId)[groupId];
      return typeof stored === "boolean" ? stored : defaultOpen;
    },
    () => defaultOpen,
  );

  const onOpenChange = useCallback(
    (next: boolean) => {
      writeGroupOpen(catalogId, groupId, next);
    },
    [catalogId, groupId],
  );

  return [open, onOpenChange] as const;
}
