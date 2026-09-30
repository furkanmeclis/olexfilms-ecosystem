import type { ListStorageParams } from "@/features/storage/types";

export const storageKeys = {
  all: ["storage"] as const,
  lists: () => [...storageKeys.all, "list"] as const,
  list: (params: ListStorageParams) =>
    [...storageKeys.lists(), params] as const,
  details: () => [...storageKeys.all, "detail"] as const,
  detail: (key: string) => [...storageKeys.details(), key] as const,
  usage: () => [...storageKeys.all, "usage"] as const,
  versions: (key: string) => [...storageKeys.all, "versions", key] as const,
  activity: (key: string) => [...storageKeys.all, "activity", key] as const,
  shares: (key: string) => [...storageKeys.all, "shares", key] as const,
  links: (key: string) => [...storageKeys.all, "links", key] as const,
};
