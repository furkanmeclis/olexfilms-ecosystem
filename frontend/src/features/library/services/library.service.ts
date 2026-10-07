import { apiConfig } from "@/config/api";
import { emitApiError, parseApiError } from "@/lib/api";
import { platformRequest } from "@/lib/api/platform-request";
import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type LibraryFolder = Schemas["LibraryFolder"];
export type LibraryFolderInput = Schemas["LibraryFolderInput"];
export type LibraryItem = Schemas["LibraryItem"];
export type LibraryItemInput = Schemas["LibraryItemInput"];
export type LibraryVersion = Schemas["LibraryVersion"];
export type LibraryAccessLevel = Schemas["LibraryAccessLevel"];
export type LibraryDownload = Schemas["LibraryDownload"];

export type LibraryItemPage = {
  items: LibraryItem[];
  total: number;
  limit: number;
  offset: number;
};

/**
 * `GET /v1/library` (TEC-331/TEC-333): folder, tag (CSV, any of),
 * access_level (CSV), updated_from/_to, q, locale, sort, limit, offset.
 */
export type LibraryListQuery = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  folder?: string;
  tag?: string;
  access_level?: string;
  updated_from?: string;
  updated_to?: string;
  locale?: string;
};

export type UploadProgress = { loaded: number; total: number };

const enc = encodeURIComponent;

function apiUrl(path: string) {
  return `${apiConfig.baseUrl.replace(/\/$/, "")}${path}`;
}

/** Multipart upload with progress events (fetch has no upload progress). */
function uploadWithProgress<T>(
  url: string,
  form: FormData,
  onProgress?: (p: UploadProgress) => void,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", url);
    xhr.withCredentials = true;
    xhr.responseType = "json";
    xhr.upload.onprogress = (event) => {
      onProgress?.({ loaded: event.loaded, total: event.total || 0 });
    };
    xhr.onload = () => {
      const payload = xhr.response ?? {};
      if (xhr.status < 200 || xhr.status >= 300) {
        const apiError = parseApiError(xhr.status, payload);
        emitApiError(apiError);
        reject(apiError);
        return;
      }
      resolve(
        payload && typeof payload === "object" && "data" in payload
          ? (payload as { data: T }).data
          : (payload as T),
      );
    };
    xhr.onerror = () => reject(new Error("Upload failed"));
    xhr.send(form);
  });
}

export const libraryService = {
  listFolders() {
    return platformRequest<{ folders: LibraryFolder[] }>(
      "GET",
      "/v1/library/folders",
    );
  },
  createFolder(body: LibraryFolderInput) {
    return platformRequest<LibraryFolder>("POST", "/v1/library/folders", {
      body,
    });
  },
  updateFolder(uuid: string, body: LibraryFolderInput) {
    return platformRequest<LibraryFolder>(
      "PATCH",
      `/v1/library/folders/${enc(uuid)}`,
      { body },
    );
  },
  deleteFolder(uuid: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/library/folders/${enc(uuid)}`,
    );
  },
  listItems(params: LibraryListQuery) {
    return platformRequest<LibraryItemPage>("GET", "/v1/library", {
      query: params,
    });
  },
  createItem(body: LibraryItemInput) {
    return platformRequest<LibraryItem>("POST", "/v1/library/items", {
      body,
    });
  },
  updateItem(uuid: string, body: LibraryItemInput) {
    return platformRequest<LibraryItem>(
      "PATCH",
      `/v1/library/items/${enc(uuid)}`,
      { body },
    );
  },
  archiveItem(uuid: string) {
    return platformRequest<{ archived: boolean }>(
      "DELETE",
      `/v1/library/items/${enc(uuid)}`,
    );
  },
  listVersions(uuid: string) {
    return platformRequest<{ versions: LibraryVersion[] }>(
      "GET",
      `/v1/library/items/${enc(uuid)}/versions`,
    );
  },
  uploadVersion(
    uuid: string,
    input: { file: File; locale: string },
    onProgress?: (p: UploadProgress) => void,
  ) {
    const form = new FormData();
    form.append("locale", input.locale);
    form.append("file", input.file);
    return uploadWithProgress<LibraryVersion>(
      apiUrl(`/v1/library/items/${enc(uuid)}/versions`),
      form,
      onProgress,
    );
  },
  download(versionUuid: string) {
    return platformRequest<LibraryDownload>(
      "GET",
      `/v1/library/versions/${enc(versionUuid)}/download`,
    );
  },
};

export const libraryKeys = {
  all: ["library"] as const,
  folders: () => [...libraryKeys.all, "folders"] as const,
  items: () => [...libraryKeys.all, "items"] as const,
  list: (params: LibraryListQuery) => [...libraryKeys.items(), params] as const,
  versions: (uuid: string) => [...libraryKeys.all, "versions", uuid] as const,
};
