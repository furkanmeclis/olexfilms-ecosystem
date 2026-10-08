import type { ServerListQuery } from "@/components/entity";
import type { components } from "@/generated/api";
import { apiClient, isApiError, platformRequest, unwrap } from "@/lib/api";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";

type Schemas = components["schemas"];

export type FleetListItem = Schemas["FleetListItem"];
export type FleetLink = Schemas["FleetLink"];
export type FleetLinkStatus = Schemas["FleetLinkStatus"];
export type Fleet = Schemas["Fleet"];
export type FleetCard = Schemas["FleetCard"];
export type FleetMatch = Schemas["FleetMatch"];
export type FleetOpenInput = Schemas["FleetOpenRequest"];
export type FleetUser = Schemas["FleetUser"];
export type FleetVehicle = Schemas["FleetVehicle"];
export type FleetStatement = Schemas["FleetStatement"];
export type FleetStatementLine = Schemas["FleetStatementLine"];
export type FleetReport = Schemas["FleetReport"];
export type FleetPlanRequest = Schemas["FleetServicePlanRequest"];
export type FleetPlanPreview = Schemas["FleetServicePlanPreview"];
export type FleetPlanAppointment = Schemas["FleetPlanAppointment"];
export type FleetPlanWarning = Schemas["FleetServicePlanWarning"];
export type FleetPlan = Schemas["FleetServicePlan"];
export type FleetPlanSummary = Schemas["FleetServicePlanSummary"];
export type FleetPlanIntake = Schemas["FleetServicePlanIntake"];
export type FleetPlanIntakeRow = FleetPlanIntake["results"][number];
export type ExportJob = Schemas["ExportJob"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export const FLEET_LINK_STATUSES: FleetLinkStatus[] = [
  "pending",
  "active",
  "ended",
];
export const FLEET_USER_STATUSES = ["active", "disabled"] as const;
export const FLEET_PLAN_STATUSES = ["scheduled", "cancelled"] as const;
export const FLEET_REPORT_STATUSES = ["pending", "ready", "failed"] as const;
export const FLEET_REPORT_KINDS = ["monthly", "quarterly"] as const;
export const FLEET_STATEMENT_KINDS = [
  "service_income",
  "collection",
  "other",
] as const;
export const FLEET_STATEMENT_EXPORT_FORMATS = ["pdf", "xlsx"] as const;
export type FleetStatementExportFormat =
  (typeof FLEET_STATEMENT_EXPORT_FORMATS)[number];

/** Import resource of the fleet vehicle import (I/O engine). */
export const FLEET_VEHICLE_IMPORT_RESOURCE = "tenant.fleet.vehicles" as const;

const enc = encodeURIComponent;
const base = (uuid: string) => `/v1/fleets/${enc(uuid)}`;

/** Errors the fleet screens show in place are not toasted globally. */
const SILENT = { silent: true } as const;

export const fleetKeys = {
  all: ["fleets"] as const,
  list: (params: ServerListQuery) => ["fleets", "list", params] as const,
  card: (uuid: string) => ["fleets", "card", uuid] as const,
  vehicles: (uuid: string, params: ServerListQuery) =>
    ["fleets", uuid, "vehicles", params] as const,
  vehiclePicker: (uuid: string, q: string) =>
    ["fleets", uuid, "vehicle-picker", q] as const,
  users: (uuid: string, params: ServerListQuery) =>
    ["fleets", uuid, "users", params] as const,
  primaryUser: (uuid: string) => ["fleets", uuid, "primary-user"] as const,
  statement: (uuid: string, from: string, to: string) =>
    ["fleets", uuid, "statement", from, to] as const,
  reports: (uuid: string, params: ServerListQuery) =>
    ["fleets", uuid, "reports", params] as const,
  plans: (uuid: string, params: ServerListQuery) =>
    ["fleets", uuid, "plans", params] as const,
  plan: (uuid: string, plan: string) => ["fleets", uuid, "plan", plan] as const,
  exportJob: (uuid: string) => ["fleets", "export-job", uuid] as const,
};

/**
 * The fleet named by a 409 FLEET_ALREADY_EXISTS (data of the error body),
 * or null for any other error.
 */
export function fleetMatchFromError(error: unknown): FleetMatch | null {
  if (!isApiError(error) || error.code !== "FLEET_ALREADY_EXISTS") return null;
  const data = (error.body as { data?: FleetMatch } | undefined)?.data;
  return data?.fleet_uuid ? data : null;
}

/** 409 FLEET_SERVICE_PLAN_STALE: capacity changed after the preview. */
export function isStalePlanError(error: unknown): boolean {
  return isApiError(error) && error.code === "FLEET_SERVICE_PLAN_STALE";
}

/** Fleet management API (TEC-473/475/476/477) behind the BFF. */
export const fleetsService = {
  list(params: ServerListQuery) {
    return platformRequest<Page<FleetListItem>>("GET", "/v1/fleets", {
      query: params,
    });
  },

  /** The brand's fleet of a VKN/TCKN, or null when there is none (404). */
  async lookup(taxNumber: string): Promise<FleetMatch | null> {
    try {
      return await unwrap<FleetMatch>(
        await apiClient.GET("/v1/fleets/lookup", {
          params: { query: { tax_number: taxNumber } },
        }),
        SILENT,
      );
    } catch (error) {
      if (isApiError(error) && error.status === 404) return null;
      throw error;
    }
  },

  async open(body: FleetOpenInput) {
    return unwrap<Fleet>(await apiClient.POST("/v1/fleets", { body }), SILENT);
  },

  async requestLink(uuid: string) {
    return unwrap<Fleet>(
      await apiClient.POST("/v1/fleets/{uuid}/links", {
        params: { path: { uuid } },
      }),
      SILENT,
    );
  },

  card(uuid: string) {
    return platformRequest<FleetCard>("GET", base(uuid));
  },

  listVehicles(uuid: string, params: ServerListQuery) {
    return platformRequest<Page<FleetVehicle>>(
      "GET",
      `${base(uuid)}/vehicles`,
      { query: params },
    );
  },

  removeVehicle(uuid: string, vehicleUuid: string) {
    return platformRequest<void>(
      "DELETE",
      `${base(uuid)}/vehicles/${enc(vehicleUuid)}`,
    );
  },

  listUsers(uuid: string, params: ServerListQuery) {
    return platformRequest<Page<FleetUser>>("GET", `${base(uuid)}/users`, {
      query: params,
    });
  },

  inviteUser(
    uuid: string,
    body: { email: string; name: string; surname?: string },
  ) {
    return platformRequest<FleetUser>("POST", `${base(uuid)}/users`, {
      body,
    });
  },

  disableUser(uuid: string, userUuid: string) {
    return platformRequest<void>(
      "POST",
      `${base(uuid)}/users/${enc(userUuid)}/disable`,
    );
  },

  statement(uuid: string, periodFrom: string, periodTo: string) {
    return platformRequest<FleetStatement>("GET", `${base(uuid)}/statement`, {
      query: { period_from: periodFrom, period_to: periodTo },
    });
  },

  exportStatement(
    uuid: string,
    body: {
      format: FleetStatementExportFormat;
      period_from: string;
      period_to: string;
      locale?: string;
    },
  ) {
    return platformRequest<ExportJob>(
      "POST",
      `${base(uuid)}/statement/export`,
      { body },
    );
  },

  getExport(jobUuid: string) {
    return platformRequest<ExportJob>(
      "GET",
      `/v1/tenant/exports/${enc(jobUuid)}`,
    );
  },

  async downloadExport(job: ExportJob) {
    const { blob, filename } = await platformDownloadFile(
      `/v1/tenant/exports/${enc(job.uuid)}/download`,
    );
    triggerBrowserDownload(blob, filename ?? `fleet-statement.${job.format}`);
  },

  listReports(uuid: string, params: ServerListQuery) {
    return platformRequest<Page<FleetReport>>("GET", `${base(uuid)}/reports`, {
      query: params,
    });
  },

  requestReport(
    uuid: string,
    body: { period_kind: "monthly" | "quarterly"; period: string },
  ) {
    return platformRequest<FleetReport>("POST", `${base(uuid)}/reports`, {
      body,
    });
  },

  async downloadReport(uuid: string, report: FleetReport) {
    const { blob, filename } = await platformDownloadFile(
      `${base(uuid)}/reports/${enc(report.uuid)}/file`,
    );
    triggerBrowserDownload(
      blob,
      filename ??
        `fleet-report-${report.period_kind}-${report.period_start}.pdf`,
    );
  },

  listPlans(uuid: string, params: ServerListQuery) {
    return platformRequest<Page<FleetPlanSummary>>(
      "GET",
      `${base(uuid)}/service-plans`,
      { query: params },
    );
  },

  getPlan(uuid: string, plan: string) {
    return platformRequest<FleetPlan>(
      "GET",
      `${base(uuid)}/service-plans/${enc(plan)}`,
    );
  },

  async previewPlan(uuid: string, body: FleetPlanRequest) {
    return unwrap<FleetPlanPreview>(
      await apiClient.POST("/v1/fleets/{uuid}/service-plans/preview", {
        params: { path: { uuid } },
        body,
      }),
      SILENT,
    );
  },

  async createPlan(
    uuid: string,
    body: FleetPlanRequest,
    idempotencyKey: string,
  ) {
    return unwrap<FleetPlan>(
      await apiClient.POST("/v1/fleets/{uuid}/service-plans", {
        params: {
          path: { uuid },
          header: { "Idempotency-Key": idempotencyKey },
        },
        body,
      }),
      SILENT,
    );
  },

  cancelPlan(uuid: string, plan: string, reason?: string) {
    return platformRequest<FleetPlan>(
      "POST",
      `${base(uuid)}/service-plans/${enc(plan)}/cancel`,
      { body: reason ? { reason } : {} },
    );
  },

  startIntake(uuid: string, plan: string, appointmentUuids: string[]) {
    return platformRequest<FleetPlanIntake>(
      "POST",
      `${base(uuid)}/service-plans/${enc(plan)}/start-intake`,
      { body: { appointment_uuids: appointmentUuids } },
    );
  },
};
