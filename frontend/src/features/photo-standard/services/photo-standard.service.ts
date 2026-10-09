import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { emitApiError, parseApiError } from "@/lib/api/errors";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type PhotoAngle = Schemas["PhotoStandardAngle"];
export type PhotoAngleInput = Schemas["PhotoStandardAngleInput"];
export type PhotoOverridesInput = Schemas["PhotoStandardOverridesInput"];
export type IntakePhoto = Schemas["IntakePhoto"];
export type IntakePhotoAngle = Schemas["IntakePhotoAngle"];
export type IntakePhotoList = Schemas["IntakePhotoList"];

const enc = encodeURIComponent;

/** Browser URL of an API path (photo and example urls go through the BFF). */
export function photoSrc(path: string): string {
  return `${apiConfig.baseUrl.replace(/\/$/, "")}${path}`;
}

/**
 * Multipart POST with upload progress (fetch has none): resolves the
 * envelope data, rejects with the parsed ApiError.
 */
function uploadWithProgress<T>(
  path: string,
  file: File,
  onProgress?: (percent: number) => void,
): Promise<T> {
  const form = new FormData();
  form.append("image", file);
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", photoSrc(path));
    xhr.withCredentials = true;
    xhr.responseType = "json";
    xhr.upload.onprogress = (event) => {
      if (onProgress && event.total > 0) {
        onProgress(Math.round((event.loaded / event.total) * 100));
      }
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

/**
 * Photo standard API (TEC-498, TEC-499, TEC-500): central angles, the
 * organization overrides and the intake photos of a service.
 */
export const photoStandardService = {
  listAngles() {
    return platformRequest<{ items: PhotoAngle[] }>(
      "GET",
      "/v1/platform/photo-standard/angles",
    );
  },
  createAngle(body: PhotoAngleInput) {
    return platformRequest<PhotoAngle>(
      "POST",
      "/v1/platform/photo-standard/angles",
      { body },
    );
  },
  updateAngle(uuid: string, body: PhotoAngleInput) {
    return platformRequest<PhotoAngle>(
      "PUT",
      `/v1/platform/photo-standard/angles/${enc(uuid)}`,
      { body },
    );
  },
  deleteAngle(uuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/platform/photo-standard/angles/${enc(uuid)}`,
    );
  },
  uploadExample(uuid: string, file: File) {
    const form = new FormData();
    form.append("image", file);
    return platformFormRequest<PhotoAngle>(
      "POST",
      `/v1/platform/photo-standard/angles/${enc(uuid)}/example`,
      form,
    );
  },
  getOverrides(organizationUuid: string) {
    return platformRequest<{ items: PhotoAngle[] }>(
      "GET",
      "/v1/photo-standard/overrides",
      { query: { organization_uuid: organizationUuid } },
    );
  },
  putOverrides(body: PhotoOverridesInput) {
    return platformRequest<{ items: PhotoAngle[] }>(
      "PUT",
      "/v1/photo-standard/overrides",
      { body },
    );
  },
  intake(serviceUuid: string) {
    return platformRequest<IntakePhotoList>(
      "GET",
      `/v1/services/${enc(serviceUuid)}/intake-photos`,
    );
  },
  uploadIntake(
    serviceUuid: string,
    angleKey: string,
    file: File,
    onProgress?: (percent: number) => void,
  ) {
    return uploadWithProgress<IntakePhoto>(
      `/v1/services/${enc(serviceUuid)}/intake-photos/${enc(angleKey)}`,
      file,
      onProgress,
    );
  },
};

export const photoStandardKeys = {
  all: ["photo-standard"] as const,
  angles: ["photo-standard", "angles"] as const,
  overrides: (orgUuid: string) =>
    ["photo-standard", "overrides", orgUuid] as const,
  intake: (serviceUuid: string) =>
    ["photo-standard", "intake", serviceUuid] as const,
};
