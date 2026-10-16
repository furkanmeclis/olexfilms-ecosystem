import type {
  ImportFormat,
  ImportJob,
  ExportJobScope,
} from "@/features/io/types";
import { apiConfig } from "@/config/api";
import {
  parseContentDispositionFilename,
  platformDownloadRequest,
  platformFormRequest,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";
import type { ListJobsParams } from "@/features/io/services/exports.service";

export type ListJobsResult = {
  items: ImportJob[];
  total: number;
  limit: number;
  offset: number;
};

/** Error report download: `missing` when the endpoint is not there (404). */
export type ImportErrorReport =
  { kind: "file"; blob: Blob; filename: string | null } | { kind: "missing" };

function importsBase(scope: ExportJobScope = "platform") {
  return scope === "tenant" ? "/v1/tenant/imports" : "/v1/platform/imports";
}

export const importsService = {
  /** `extra`: resource-specific options sent with the file (wizard extraOptions). */
  async upload(
    path: string,
    file: File,
    format: ImportFormat,
    locale: string,
    extra?: Record<string, string>,
  ) {
    const form = new FormData();
    form.append("file", file);
    form.append("format", format);
    form.append("locale", locale);
    for (const [key, value] of Object.entries(extra ?? {})) {
      if (!["file", "format", "locale"].includes(key)) form.append(key, value);
    }
    return platformFormRequest<ImportJob>("POST", path, form);
  },

  async downloadSample(path: string, format: ImportFormat, locale: string) {
    return platformDownloadRequest(path, { format, locale });
  },

  async updateMapping(
    uuid: string,
    body: {
      mapping: Record<string, string>;
      defaults?: Record<string, string>;
    },
    scope: ExportJobScope = "platform",
  ) {
    return platformRequest<ImportJob>(
      "PATCH",
      `${importsBase(scope)}/${uuid}/mapping`,
      { body },
    );
  },

  async preview(uuid: string, scope: ExportJobScope = "platform") {
    return platformRequest<ImportJob>(
      "POST",
      `${importsBase(scope)}/${uuid}/preview`,
    );
  },

  async confirm(uuid: string, scope: ExportJobScope = "platform") {
    return platformRequest<ImportJob>(
      "POST",
      `${importsBase(scope)}/${uuid}/confirm`,
    );
  },

  async rollback(uuid: string, scope: ExportJobScope = "platform") {
    return platformRequest<ImportJob>(
      "POST",
      `${importsBase(scope)}/${uuid}/rollback`,
    );
  },

  /**
   * Job list: `sort`, `q`, CSV `status` / `resource` / `format`,
   * `created_from` / `created_to` (TEC-365).
   */
  async list(params: ListJobsParams, scope: ExportJobScope = "platform") {
    return platformRequest<ListJobsResult>("GET", importsBase(scope), {
      query: {
        limit: params.limit,
        offset: params.offset,
        sort: params.sort,
        q: params.q,
        status: params.status,
        resource: params.resource,
        format: params.format,
        created_from: params.created_from,
        created_to: params.created_to,
      },
    });
  },

  /**
   * XLSX of the rows that are not imported, with their errors (TEC-757,
   * `GET /imports/{uuid}/error-report`; requested through the BFF by hand
   * until it is in the generated API).
   */
  async errorReport(
    uuid: string,
    scope: ExportJobScope = "platform",
  ): Promise<ImportErrorReport> {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(
      `${base}${importsBase(scope)}/${encodeURIComponent(uuid)}/error-report`,
      { credentials: "include" },
    );
    if (response.status === 404 || response.status === 405) {
      return { kind: "missing" };
    }
    if (!response.ok) {
      throw new Error(`Error report failed (${response.status})`);
    }
    return {
      kind: "file",
      blob: await response.blob(),
      filename: parseContentDispositionFilename(
        response.headers.get("Content-Disposition"),
      ),
    };
  },

  async get(uuid: string, scope: ExportJobScope = "platform") {
    return platformRequest<ImportJob>("GET", `${importsBase(scope)}/${uuid}`);
  },
};
