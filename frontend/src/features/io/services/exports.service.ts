import type {
  ExportFormat,
  ExportJob,
  ExportJobScope,
} from "@/features/io/types";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";
import { exportDownloadFilename } from "@/features/io/lib/display";

export type ListJobsParams = {
  limit?: number;
  offset?: number;
  sort?: string;
  q?: string;
  status?: string;
  resource?: string;
  format?: string;
  created_from?: string;
  created_to?: string;
};
export type ListJobsResult = {
  items: ExportJob[];
  total: number;
  limit: number;
  offset: number;
};

function exportsBase(scope: ExportJobScope = "platform") {
  return scope === "tenant" ? "/v1/tenant/exports" : "/v1/platform/exports";
}

export const exportsService = {
  async request(
    path: string,
    body: {
      format: ExportFormat;
      query?: Record<string, string>;
      locale?: string;
    },
  ) {
    return platformRequest<ExportJob>("POST", path, { body });
  },

  /**
   * Job list: `sort`, `q`, CSV `status` / `resource` / `format`,
   * `created_from` / `created_to` (TEC-365).
   */
  async list(params: ListJobsParams, scope: ExportJobScope = "platform") {
    return platformRequest<ListJobsResult>("GET", exportsBase(scope), {
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
    return platformRequest<ExportJob>("GET", `${exportsBase(scope)}/${uuid}`);
  },

  async fetchFile(uuid: string, scope: ExportJobScope = "platform") {
    return platformDownloadFile(`${exportsBase(scope)}/${uuid}/download`);
  },

  async download(job: ExportJob, scope: ExportJobScope = "platform") {
    const { blob, filename } = await this.fetchFile(job.uuid, scope);
    const name =
      filename ??
      exportDownloadFilename(
        job.resource,
        job.format,
        new Date(job.created_at),
      );
    triggerBrowserDownload(blob, name);
  },
};
