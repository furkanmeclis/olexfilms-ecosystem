import type {
  ImportFormat,
  ImportJob,
  ExportJobScope,
} from "@/features/io/types";
import {
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

function importsBase(scope: ExportJobScope = "platform") {
  return scope === "tenant" ? "/v1/tenant/imports" : "/v1/platform/imports";
}

export const importsService = {
  async upload(path: string, file: File, format: ImportFormat, locale: string) {
    const form = new FormData();
    form.append("file", file);
    form.append("format", format);
    form.append("locale", locale);
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

  async get(uuid: string, scope: ExportJobScope = "platform") {
    return platformRequest<ImportJob>("GET", `${importsBase(scope)}/${uuid}`);
  },
};
