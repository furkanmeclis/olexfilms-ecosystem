import { apiConfig } from "@/config/api";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { parseApiError, emitApiError } from "@/lib/api";
import { platformRequest } from "@/lib/api/platform-request";
import type {
  CreateStorageLinkInput,
  CreateStorageShareInput,
  ListStorageParams,
  StorageActivity,
  StorageLink,
  StorageListResult,
  StorageObject,
  StorageService,
  StorageShare,
  StorageUsage,
  StorageVersion,
  UploadSession,
} from "@/features/storage/types";

function queryOf(input: ListStorageParams) {
  return {
    prefix: input.prefix,
    view: input.view,
    q: input.q,
    kind: input.kind,
    access: input.access,
    sort: input.sort,
    limit: input.limit,
    offset: input.offset,
    modified_from: input.modified_from,
    modified_to: input.modified_to,
    recursive: input.recursive ? "true" : undefined,
  };
}

function absoluteShareUrl(path: string) {
  if (!path) return "";
  if (path.startsWith("http")) return path;
  const origin = typeof window === "undefined" ? "" : window.location.origin;
  return `${origin}/api${path.startsWith("/") ? path : `/${path}`}`;
}

function appOrigin() {
  if (typeof window === "undefined") return "";
  return window.location.origin;
}

function extractSignedToken(tokenOrUrl: string) {
  if (!tokenOrUrl.includes("/")) return tokenOrUrl;
  const match = /\/s\/([^/?#]+)/.exec(tokenOrUrl);
  return match?.[1] ?? tokenOrUrl;
}

function uploadWithProgress(
  url: string,
  form: FormData,
  onProgress?: (p: { loaded: number; total: number }) => void,
  signal?: AbortSignal,
): Promise<StorageObject> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", url);
    xhr.withCredentials = true;
    xhr.responseType = "json";
    xhr.upload.onprogress = (event) => {
      if (!onProgress) return;
      onProgress({ loaded: event.loaded, total: event.total || 0 });
    };
    xhr.onload = () => {
      const payload = xhr.response ?? {};
      if (xhr.status < 200 || xhr.status >= 300) {
        const apiError = parseApiError(xhr.status, payload);
        emitApiError(apiError);
        reject(apiError);
        return;
      }
      const data =
        payload && typeof payload === "object" && "data" in payload
          ? (payload as { data: StorageObject }).data
          : (payload as StorageObject);
      resolve(data);
    };
    xhr.onerror = () => reject(new Error("Upload failed"));
    xhr.onabort = () => reject(new DOMException("Aborted", "AbortError"));
    signal?.addEventListener("abort", () => xhr.abort());
    xhr.send(form);
  });
}

export function createApiStorageAdapter(): StorageService {
  return {
    listFiles(input) {
      return platformRequest<StorageListResult>(
        "GET",
        "/v1/platform/storage/objects",
        { query: queryOf(input) },
      );
    },
    getFile(key) {
      return platformRequest<StorageObject>(
        "GET",
        "/v1/platform/storage/objects/item",
        { query: { key } },
      );
    },
    getUsage() {
      return platformRequest<StorageUsage>("GET", "/v1/platform/storage/usage");
    },
    createFolder(input) {
      return platformRequest<StorageObject>(
        "POST",
        "/v1/platform/storage/folders",
        { body: input },
      );
    },
    async uploadFile(input) {
      const form = new FormData();
      form.set("file", input.file);
      if (input.prefix) form.set("prefix", input.prefix);
      if (input.key) form.set("key", input.key);
      const url = `${apiConfig.baseUrl.replace(/\/$/, "")}/v1/platform/storage/uploads`;
      return uploadWithProgress(url, form, input.onProgress, input.signal);
    },
    createUploadSession(input) {
      return platformRequest<UploadSession>(
        "POST",
        "/v1/platform/storage/uploads/session",
        { body: input },
      );
    },
    completeUpload(key) {
      return platformRequest<StorageObject>(
        "POST",
        "/v1/platform/storage/uploads/complete",
        { body: { key } },
      );
    },
    copyFile(input) {
      return platformRequest<StorageObject>(
        "POST",
        "/v1/platform/storage/objects/copy",
        { body: input },
      );
    },
    moveFile(input) {
      return platformRequest<StorageObject>(
        "POST",
        "/v1/platform/storage/objects/move",
        { body: input },
      );
    },
    renameFile(input) {
      return platformRequest<StorageObject>(
        "POST",
        "/v1/platform/storage/objects/rename",
        { body: input },
      );
    },
    async deleteFile(keys) {
      await platformRequest("POST", "/v1/platform/storage/objects/delete", {
        body: { keys },
      });
    },
    async restoreFile(keys) {
      await platformRequest("POST", "/v1/platform/storage/objects/restore", {
        body: { keys },
      });
    },
    async purgeFile(keys) {
      await platformRequest("POST", "/v1/platform/storage/objects/purge", {
        body: { keys },
      });
    },
    previewUrl(key, versionId) {
      const params = new URLSearchParams({ key });
      if (versionId) params.set("version_id", versionId);
      return `${apiConfig.baseUrl.replace(/\/$/, "")}/v1/platform/storage/objects/preview?${params}`;
    },
    publicSharePageUrl(slug: string) {
      return `${appOrigin()}/share/${encodeURIComponent(slug)}`;
    },
    signedSharePageUrl(tokenOrUrl: string) {
      const token = extractSignedToken(tokenOrUrl);
      return `${appOrigin()}/share/s/${encodeURIComponent(token)}`;
    },
    publicStreamUrl(slug: string, download = false) {
      const base = `${apiConfig.baseUrl.replace(/\/$/, "")}/v1/public/storage/${encodeURIComponent(slug)}`;
      return download ? `${base}?download=1` : base;
    },
    signedStreamUrl(token: string, download = false) {
      const base = `${apiConfig.baseUrl.replace(/\/$/, "")}/v1/public/storage/s/${encodeURIComponent(token)}`;
      return download ? `${base}?download=1` : base;
    },
    async downloadFile(key, filename, versionId) {
      const { blob, filename: headerName } = await platformDownloadFile(
        "/v1/platform/storage/objects/download",
        { key, version_id: versionId },
      );
      triggerBrowserDownload(blob, headerName || filename);
    },
    async getVersions(key) {
      const data = await platformRequest<{ items: StorageVersion[] }>(
        "GET",
        "/v1/platform/storage/versions",
        { query: { key } },
      );
      return data.items;
    },
    restoreVersion(input) {
      return platformRequest<StorageObject>(
        "POST",
        "/v1/platform/storage/versions/restore",
        { body: input },
      );
    },
    async deleteVersion(input) {
      await platformRequest("POST", "/v1/platform/storage/versions/delete", {
        body: input,
      });
    },
    async getActivity(key, params) {
      return platformRequest<{ items: StorageActivity[]; total: number }>(
        "GET",
        "/v1/platform/storage/activity",
        {
          query: {
            key,
            limit: params?.limit,
            offset: params?.offset,
          },
        },
      );
    },
    async starFile(key) {
      await platformRequest("POST", "/v1/platform/storage/star", {
        body: { key },
      });
    },
    async unstarFile(key) {
      await platformRequest("DELETE", "/v1/platform/storage/star", {
        query: { key },
      });
    },
    async listShares(key) {
      const data = await platformRequest<{ items: StorageShare[] }>(
        "GET",
        "/v1/platform/storage/shares",
        { query: { key } },
      );
      return data.items;
    },
    shareFile(input: CreateStorageShareInput) {
      return platformRequest<StorageShare>(
        "POST",
        "/v1/platform/storage/shares",
        { body: input },
      );
    },
    async unshareFile(uuid) {
      await platformRequest("DELETE", `/v1/platform/storage/shares/${uuid}`);
    },
    async listLinks(key) {
      const data = await platformRequest<{ items: StorageLink[] }>(
        "GET",
        "/v1/platform/storage/links",
        { query: { key } },
      );
      return data.items.map((item) => ({
        ...item,
        url: absoluteShareUrl(item.url),
      }));
    },
    async createPublicLink(input: CreateStorageLinkInput) {
      const item = await platformRequest<StorageLink>(
        "POST",
        "/v1/platform/storage/links",
        { body: { ...input, kind: "public" } },
      );
      return { ...item, url: absoluteShareUrl(item.url) };
    },
    async createSignedUrl(input: CreateStorageLinkInput) {
      const item = await platformRequest<StorageLink>(
        "POST",
        "/v1/platform/storage/links",
        { body: { ...input, kind: "signed" } },
      );
      return { ...item, url: absoluteShareUrl(item.url) };
    },
    async revokeLink(uuid) {
      await platformRequest("DELETE", `/v1/platform/storage/links/${uuid}`);
    },
  };
}

export function createStorageService(adapter: StorageService): StorageService {
  return adapter;
}

export const storageService = createStorageService(createApiStorageAdapter());
