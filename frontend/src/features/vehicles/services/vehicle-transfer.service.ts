import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Vehicle = Schemas["Vehicle"];
export type VehicleTransfer = Schemas["VehicleTransfer"];
export type VehicleTransferVerifyInput = Schemas["VehicleTransferVerifyInput"];

const enc = encodeURIComponent;

/**
 * Vehicle detail and the ownership transfer (TEC-190): start with the new
 * owner's phone, enter the two WhatsApp codes, cancel. Through the BFF with
 * the active organization of the session.
 */
export const vehicleTransferService = {
  getVehicle(uuid: string) {
    return platformRequest<Vehicle>("GET", `/v1/vehicles/${enc(uuid)}`);
  },
  listTransfers(vehicleUuid: string) {
    return platformRequest<{ items: VehicleTransfer[] }>(
      "GET",
      `/v1/vehicles/${enc(vehicleUuid)}/transfers`,
    );
  },
  start(vehicleUuid: string, phone: string) {
    return platformRequest<VehicleTransfer>(
      "POST",
      `/v1/vehicles/${enc(vehicleUuid)}/transfers`,
      { body: { phone } },
    );
  },
  verify(transferUuid: string, body: VehicleTransferVerifyInput) {
    return platformRequest<VehicleTransfer>(
      "POST",
      `/v1/vehicle-transfers/${enc(transferUuid)}/verify`,
      { body },
    );
  },
  cancel(transferUuid: string) {
    return platformRequest<VehicleTransfer>(
      "POST",
      `/v1/vehicle-transfers/${enc(transferUuid)}/cancel`,
    );
  },
};

export const vehicleKeys = {
  vehicle: (uuid: string) => ["vehicles", "detail", uuid] as const,
  transfers: (uuid: string) => ["vehicles", "transfers", uuid] as const,
};
