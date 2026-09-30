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

  async list(
    params: { limit?: number; offset?: number },
    scope: ExportJobScope = "platform",
  ) {
    return platformRequest<ListJobsResult>("GET", exportsBase(scope), {
      query: params,
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
