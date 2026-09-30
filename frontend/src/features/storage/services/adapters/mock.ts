import { createStorageService } from "@/features/storage/services/storage.service";
import type {
  StorageListResult,
  StorageObject,
  StorageService,
  StorageUsage,
} from "@/features/storage/types";

function folder(key: string, name: string): StorageObject {
  return {
    id: key,
    name,
    key,
    prefix: key.includes("/") ? key.slice(0, key.lastIndexOf("/") + 1) : "",
    bucket: "app",
    kind: "folder",
    file_kind: "folder",
    size: 0,
    mime_type: "application/x-directory",
    is_public: false,
    is_starred: false,
    is_shared: false,
    access: "private",
    metadata: {},
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  };
}

const seed: StorageObject[] = [
  folder("Projects/", "Projects"),
  {
    ...folder("readme.md", "readme.md"),
    kind: "file",
    file_kind: "code",
    mime_type: "text/markdown",
    size: 2048,
    key: "readme.md",
    id: "readme.md",
    prefix: "",
    name: "readme.md",
  },
];

export function createMockStorageAdapter(): StorageService {
  let items = [...seed];
  const svc: StorageService = {
    async listFiles(input) {
      const prefix = input.prefix ?? "";
      const view = input.view ?? "all";
      let filtered = items.filter((item) => {
        if (view === "starred") return item.is_starred;
        if (view === "public") return item.is_public;
        if (view === "trash") return Boolean(item.trash_uuid);
        if (prefix) return item.prefix === prefix || item.key === prefix;
        return item.prefix === "";
      });
      if (input.q) {
        const q = input.q.toLowerCase();
        filtered = filtered.filter((item) =>
          item.name.toLowerCase().includes(q),
        );
      }
      const limit = input.limit ?? 20;
      const offset = input.offset ?? 0;
      return {
        items: filtered.slice(offset, offset + limit),
        total: filtered.length,
        limit,
        offset,
        prefix,
        view,
      } satisfies StorageListResult;
    },
    async getFile(key) {
      const found = items.find((item) => item.key === key);
      if (!found) throw new Error("not found");
      return found;
    },
    async getUsage() {
      const files = items.filter((item) => item.kind === "file");
      const used = files.reduce((sum, item) => sum + item.size, 0);
      return {
        total_bytes: used,
        used_bytes: used,
        available_bytes: 0,
        quota_bytes: 0,
        total_files: files.length,
        total_folders: items.filter((item) => item.kind === "folder").length,
        by_kind: [],
        largest: files.slice(0, 5),
        recent: files.slice(0, 5),
      } satisfies StorageUsage;
    },
    async createFolder(input) {
      const key = `${input.prefix ?? ""}${input.name}/`;
      const obj = folder(key, input.name);
      items = [...items, obj];
      return obj;
    },
    async uploadFile(input) {
      const key = input.key || `${input.prefix ?? ""}${input.file.name}`;
      const obj: StorageObject = {
        id: key,
        name: input.file.name,
        key,
        prefix: input.prefix ?? "",
        bucket: "app",
        kind: "file",
        file_kind: "unknown",
        size: input.file.size,
        mime_type: input.file.type,
        is_public: false,
        is_starred: false,
        is_shared: false,
        access: "private",
        metadata: {},
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      items = [...items.filter((item) => item.key !== key), obj];
      return obj;
    },
    async createUploadSession() {
      return {
        session_id: "mock",
        key: "mock",
        method: "post",
        upload_url: "/v1/platform/storage/uploads",
        expires_in: 3600,
      };
    },
    async completeUpload(key) {
      return svc.getFile(key);
    },
    async copyFile(input) {
      const src = await svc.getFile(input.source_key);
      const copy = { ...src, id: input.dest_key, key: input.dest_key };
      items = [...items, copy];
      return copy;
    },
    async moveFile(input) {
      const src = await svc.getFile(input.source_key);
      const moved = { ...src, id: input.dest_key, key: input.dest_key };
      items = items.filter((item) => item.key !== src.key).concat(moved);
      return moved;
    },
    async renameFile(input) {
      return svc.moveFile({
        source_key: input.key,
        dest_key: input.name,
      });
    },
    async deleteFile(keys) {
      items = items.map((item) =>
        keys.includes(item.key)
          ? {
              ...item,
              trash_uuid: item.key,
              trash_expires_at: new Date().toISOString(),
            }
          : item,
      );
    },
    async restoreFile(keys) {
      items = items.map((item) =>
        keys.includes(item.key)
          ? { ...item, trash_uuid: undefined, trash_expires_at: undefined }
          : item,
      );
    },
    async purgeFile(keys) {
      items = items.filter((item) => !keys.includes(item.key));
    },
    previewUrl() {
      return "";
    },
    publicSharePageUrl(slug) {
      return `/share/${slug}`;
    },
    signedSharePageUrl(tokenOrUrl) {
      return `/share/s/${tokenOrUrl}`;
    },
    publicStreamUrl(slug, download) {
      return download
        ? `/api/v1/public/storage/${slug}?download=1`
        : `/api/v1/public/storage/${slug}`;
    },
    signedStreamUrl(token, download) {
      return download
        ? `/api/v1/public/storage/s/${token}?download=1`
        : `/api/v1/public/storage/s/${token}`;
    },
    async downloadFile() {},
    async getVersions() {
      return [];
    },
    async restoreVersion() {
      throw new Error("not implemented");
    },
    async deleteVersion() {},
    async getActivity() {
      return { items: [], total: 0 };
    },
    async starFile(key) {
      items = items.map((item) =>
        item.key === key ? { ...item, is_starred: true } : item,
      );
    },
    async unstarFile(key) {
      items = items.map((item) =>
        item.key === key ? { ...item, is_starred: false } : item,
      );
    },
    async listShares() {
      return [];
    },
    async shareFile() {
      throw new Error("not implemented");
    },
    async unshareFile() {},
    async listLinks() {
      return [];
    },
    async createPublicLink() {
      throw new Error("not implemented");
    },
    async createSignedUrl() {
      throw new Error("not implemented");
    },
    async revokeLink() {},
  };
  return svc;
}

export const mockStorageService = createStorageService(
  createMockStorageAdapter(),
);
