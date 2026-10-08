import type { components } from "@/generated/api";
import {
  platformDownloadFile,
  platformFormRequest,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Showcase = Schemas["Showcase"];
export type ShowcaseInput = Schemas["ShowcaseInput"];
export type ShowcaseService = Schemas["ShowcaseService"];
export type ShowcaseServiceInput = Schemas["ShowcaseServiceInput"];
export type ShowcasePhoto = Schemas["ShowcasePhoto"];
export type ShowcaseStatus = Schemas["ShowcaseStatus"];
export type ShowcaseReviewItem = Schemas["ShowcaseReviewItem"];
export type ShowcaseReviewInput = Schemas["ShowcaseReviewInput"];
export type ShowcaseGoogleRatingInput = Schemas["ShowcaseGoogleRatingInput"];

export type ShowcaseTarget = { org?: string };

export type ShowcaseReviewQuery = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  updated_from?: string;
  updated_to?: string;
};

export type ShowcaseReviewPage = {
  items: ShowcaseReviewItem[];
  total: number;
  limit: number;
  offset: number;
};

const enc = encodeURIComponent;

function targetQuery(target?: ShowcaseTarget) {
  return target?.org ? { org: target.org } : undefined;
}

export const showcaseKeys = {
  all: ["showcase"] as const,
  editor: (org?: string) => ["showcase", "editor", org ?? "self"] as const,
  review: (params: ShowcaseReviewQuery) =>
    ["showcase", "review", params] as const,
  platform: (orgUuid: string) => ["showcase", "platform", orgUuid] as const,
};

export const showcaseService = {
  get(target?: ShowcaseTarget) {
    return platformRequest<Showcase>("GET", "/v1/showcase", {
      query: targetQuery(target),
    });
  },
  save(body: ShowcaseInput, target?: ShowcaseTarget) {
    return platformRequest<Showcase>("PUT", "/v1/showcase", {
      query: targetQuery(target),
      body,
    });
  },
  submit(target?: ShowcaseTarget) {
    return platformRequest<Showcase>("POST", "/v1/showcase/submit", {
      query: targetQuery(target),
    });
  },
  setGoogleRating(body: ShowcaseGoogleRatingInput, target?: ShowcaseTarget) {
    return platformRequest<Showcase>("PUT", "/v1/showcase/google-rating", {
      query: targetQuery(target),
      body,
    });
  },
  createService(body: ShowcaseServiceInput, target?: ShowcaseTarget) {
    return platformRequest<ShowcaseService>("POST", "/v1/showcase/services", {
      query: targetQuery(target),
      body,
    });
  },
  updateService(
    uuid: string,
    body: ShowcaseServiceInput,
    target?: ShowcaseTarget,
  ) {
    return platformRequest<ShowcaseService>(
      "PUT",
      `/v1/showcase/services/${enc(uuid)}`,
      { query: targetQuery(target), body },
    );
  },
  deleteService(uuid: string, target?: ShowcaseTarget) {
    return platformRequest<void>(
      "DELETE",
      `/v1/showcase/services/${enc(uuid)}`,
      {
        query: targetQuery(target),
      },
    );
  },
  reorderServices(uuids: string[], target?: ShowcaseTarget) {
    return platformRequest<{ items: ShowcaseService[] }>(
      "PUT",
      "/v1/showcase/services/order",
      { query: targetQuery(target), body: { uuids } },
    );
  },
  uploadPhoto(
    file: File,
    caption: Record<string, string>,
    target?: ShowcaseTarget,
  ) {
    const data = new FormData();
    data.set("photo", file);
    data.set("caption", JSON.stringify(caption));
    const query = target?.org ? `?org=${enc(target.org)}` : "";
    return platformFormRequest<ShowcasePhoto>(
      "POST",
      `/v1/showcase/photos${query}`,
      data,
    );
  },
  updatePhoto(
    uuid: string,
    caption: Record<string, string>,
    target?: ShowcaseTarget,
  ) {
    return platformRequest<ShowcasePhoto>(
      "PUT",
      `/v1/showcase/photos/${enc(uuid)}`,
      { query: targetQuery(target), body: { caption } },
    );
  },
  deletePhoto(uuid: string, target?: ShowcaseTarget) {
    return platformRequest<void>("DELETE", `/v1/showcase/photos/${enc(uuid)}`, {
      query: targetQuery(target),
    });
  },
  reorderPhotos(uuids: string[], target?: ShowcaseTarget) {
    return platformRequest<{ items: ShowcasePhoto[] }>(
      "PUT",
      "/v1/showcase/photos/order",
      { query: targetQuery(target), body: { uuids } },
    );
  },
  photoUrl(uuid: string, target?: ShowcaseTarget) {
    const query = target?.org ? `?org=${enc(target.org)}` : "";
    return `/api/v1/showcase/photos/${enc(uuid)}/file${query}`;
  },
  downloadPhoto(uuid: string, target?: ShowcaseTarget) {
    return platformDownloadFile(`/v1/showcase/photos/${enc(uuid)}/file`, {
      org: target?.org,
    });
  },
  reviewList(params: ShowcaseReviewQuery) {
    return platformRequest<ShowcaseReviewPage>(
      "GET",
      "/v1/platform/showcases",
      {
        query: params,
      },
    );
  },
  platformGet(orgUuid: string) {
    return platformRequest<Showcase>(
      "GET",
      `/v1/platform/showcases/${enc(orgUuid)}`,
    );
  },
  review(orgUuid: string, body: ShowcaseReviewInput) {
    return platformRequest<Showcase>(
      "POST",
      `/v1/platform/showcases/${enc(orgUuid)}/review`,
      { body },
    );
  },
};
