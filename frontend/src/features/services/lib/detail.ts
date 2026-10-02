import type {
  Service,
  ServiceItem,
  ServiceStatus,
  ServiceStatusLog,
  ServiceWarranty,
} from "@/features/services/services/service-wizard.service";

export type Tone = "default" | "success" | "warning" | "danger";

const STATUS_TONE: Record<ServiceStatus, Tone> = {
  draft: "default",
  pending: "warning",
  processing: "warning",
  ready: "success",
  completed: "success",
  cancelled: "danger",
};

export function serviceStatusTone(status: ServiceStatus): Tone {
  return STATUS_TONE[status] ?? "default";
}

export function warrantyTone(status: ServiceWarranty["status"]): Tone {
  if (status === "active") return "success";
  if (status === "void") return "danger";
  return "default";
}

/** Status history, newest first. */
export function sortedStatusLogs(
  logs: readonly ServiceStatusLog[] | undefined,
): ServiceStatusLog[] {
  return [...(logs ?? [])].sort(
    (a, b) => Date.parse(b.created_at) - Date.parse(a.created_at),
  );
}

/** i18n key + params of an item's amount (meters, pieces or whole unit). */
export function itemAmount(item: ServiceItem): {
  key: string;
  params?: Record<string, string | number>;
} {
  if (item.kind === "partial" && item.meters) {
    return {
      key: "services.stock.amount_meters",
      params: { meters: item.meters },
    };
  }
  if (item.quantity) {
    return {
      key: "services.stock.amount_pieces",
      params: { count: item.quantity },
    };
  }
  return { key: "services.stock.amount_whole" };
}

/** Vehicle title: brand, model and year. */
export function vehicleTitle(service: Service): string {
  return [service.car_brand.name, service.car_model.name, service.model_year]
    .filter((v) => v !== null && v !== undefined && v !== "")
    .join(" ");
}

/** Customer display name (masked by the API when anonymized). */
export function customerName(service: Service): string {
  return [service.customer.name, service.customer.surname]
    .filter(Boolean)
    .join(" ");
}
