export type StorageView =
  "all" | "recent" | "starred" | "shared" | "public" | "trash";

export type StorageAccess =
  "private" | "public" | "shared" | "temporary" | "expired" | "revoked";

export type StorageFileKind =
  | "folder"
  | "pdf"
  | "image"
  | "video"
  | "audio"
  | "archive"
  | "document"
  | "spreadsheet"
  | "code"
  | "unknown";

export type StorageObject = {
  id: string;
  name: string;
  key: string;
  prefix: string;
  bucket: string;
  kind: "file" | "folder";
  file_kind: StorageFileKind;
  size: number;
  mime_type: string;
  etag?: string;
  version_id?: string;
  version?: string;
  is_public: boolean;
  is_starred: boolean;
  is_shared: boolean;
  access: StorageAccess;
  owner?: string;
  storage_class?: string;
  content_disposition?: string;
  cache_control?: string;
  metadata: Record<string, string>;
  created_at: string;
  updated_at: string;
  trash_uuid?: string;
  trash_expires_at?: string;
};

export type StorageListResult = {
  items: StorageObject[];
  total: number;
  limit: number;
  offset: number;
  prefix: string;
  view: StorageView;
};

export type ListStorageParams = {
  prefix?: string;
  view?: StorageView;
  q?: string;
  kind?: string;
  access?: string;
  sort?: string;
  limit?: number;
  offset?: number;
  modified_from?: string;
  modified_to?: string;
  recursive?: boolean;
};

export type StorageUsageKind = {
  kind: string;
  bytes: number;
  count: number;
};

export type StorageUsage = {
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
  quota_bytes: number;
  total_files: number;
  total_folders: number;
  by_kind: StorageUsageKind[];
  largest: StorageObject[];
  recent: StorageObject[];
};

export type StorageVersion = {
  version_id: string;
  label: string;
  size: number;
  etag?: string;
  is_latest: boolean;
  status: string;
  created_at: string;
  created_by?: string;
};

export type StorageActivity = {
  uuid: string;
  object_key: string;
  action: string;
  actor: string;
  payload: Record<string, unknown>;
  created_at: string;
};

export type StorageLink = {
  uuid: string;
  object_key: string;
  kind: "public" | "signed";
  slug?: string;
  url: string;
  token?: string;
  expires_at?: string;
  can_view: boolean;
  can_download: boolean;
  can_upload: boolean;
  status: "active" | "expired" | "revoked";
  created_at: string;
};

export type CreateStorageLinkInput = {
  key: string;
  kind: "public" | "signed";
  slug?: string;
  expires_in?: number;
  can_view?: boolean;
  can_download?: boolean;
  can_upload?: boolean;
};

export type StorageShare = {
  uuid: string;
  object_key: string;
  user_uuid: string;
  name: string;
  email: string;
  role: "viewer" | "editor" | "owner";
  created_at: string;
};

export type CreateStorageShareInput = {
  key: string;
  user_uuid: string;
  role: "viewer" | "editor" | "owner";
};

export type UploadSession = {
  session_id: string;
  key: string;
  method: string;
  upload_url: string;
  expires_in: number;
};

export type UploadProgressHandler = (progress: {
  loaded: number;
  total: number;
}) => void;

export type StorageService = {
  listFiles(input: ListStorageParams): Promise<StorageListResult>;
  getFile(key: string): Promise<StorageObject>;
  getUsage(): Promise<StorageUsage>;
  createFolder(input: { prefix: string; name: string }): Promise<StorageObject>;
  uploadFile(input: {
    file: File;
    prefix?: string;
    key?: string;
    onProgress?: UploadProgressHandler;
    signal?: AbortSignal;
  }): Promise<StorageObject>;
  createUploadSession(input: {
    prefix?: string;
    key?: string;
    filename?: string;
    content_type?: string;
    size?: number;
  }): Promise<UploadSession>;
  completeUpload(key: string): Promise<StorageObject>;
  copyFile(input: {
    source_key: string;
    dest_key: string;
  }): Promise<StorageObject>;
  moveFile(input: {
    source_key: string;
    dest_key: string;
  }): Promise<StorageObject>;
  renameFile(input: { key: string; name: string }): Promise<StorageObject>;
  deleteFile(keys: string[]): Promise<void>;
  restoreFile(keys: string[]): Promise<void>;
  purgeFile(keys: string[]): Promise<void>;
  previewUrl(key: string, versionId?: string): string;
  publicSharePageUrl(slug: string): string;
  signedSharePageUrl(tokenOrUrl: string): string;
  publicStreamUrl(slug: string, download?: boolean): string;
  signedStreamUrl(token: string, download?: boolean): string;
  downloadFile(
    key: string,
    filename: string,
    versionId?: string,
  ): Promise<void>;
  getVersions(key: string): Promise<StorageVersion[]>;
  restoreVersion(input: {
    key: string;
    version_id: string;
  }): Promise<StorageObject>;
  deleteVersion(input: { key: string; version_id: string }): Promise<void>;
  getActivity(
    key: string,
    params?: { limit?: number; offset?: number },
  ): Promise<{ items: StorageActivity[]; total: number }>;
  starFile(key: string): Promise<void>;
  unstarFile(key: string): Promise<void>;
  listShares(key: string): Promise<StorageShare[]>;
  shareFile(input: CreateStorageShareInput): Promise<StorageShare>;
  unshareFile(uuid: string): Promise<void>;
  listLinks(key: string): Promise<StorageLink[]>;
  createPublicLink(input: CreateStorageLinkInput): Promise<StorageLink>;
  createSignedUrl(input: CreateStorageLinkInput): Promise<StorageLink>;
  revokeLink(uuid: string): Promise<void>;
};

export type ResourceMeta = {
  resource: string;
  default_sort: string;
  capabilities: Record<string, boolean>;
};
