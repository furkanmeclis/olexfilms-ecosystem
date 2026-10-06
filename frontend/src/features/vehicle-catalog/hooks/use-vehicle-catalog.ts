"use client";

import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  vehicleCatalogService,
  type VehicleBrand,
  type VehicleBrandInput,
  type VehicleListParams,
  type VehicleModelInput,
  type VehicleModelListParams,
} from "@/features/vehicle-catalog/services/vehicle-catalog.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const vehicleCatalogKeys = {
  all: ["vehicle-catalog"] as const,
  brandLists: () => [...vehicleCatalogKeys.all, "brands"] as const,
  brandList: (params: VehicleListParams) =>
    [...vehicleCatalogKeys.brandLists(), params] as const,
  brand: (uuid: string) => [...vehicleCatalogKeys.all, "brand", uuid] as const,
  modelLists: () => [...vehicleCatalogKeys.all, "models"] as const,
  modelList: (params: VehicleModelListParams) =>
    [...vehicleCatalogKeys.modelLists(), params] as const,
  modelFacets: (brandUuid: string) =>
    [...vehicleCatalogKeys.modelLists(), "facets", brandUuid] as const,
};

export function useVehicleBrands(params: VehicleListParams) {
  return useQuery({
    queryKey: vehicleCatalogKeys.brandList(params),
    queryFn: () => vehicleCatalogService.listBrands(params),
    placeholderData: keepPreviousData,
  });
}

export function useVehicleBrand(uuid: string) {
  return useQuery({
    queryKey: vehicleCatalogKeys.brand(uuid),
    queryFn: () => vehicleCatalogService.getBrand(uuid),
    enabled: Boolean(uuid),
  });
}

export function useVehicleModels(params: VehicleModelListParams) {
  return useQuery({
    queryKey: vehicleCatalogKeys.modelList(params),
    queryFn: () => vehicleCatalogService.listModels(params),
    enabled: Boolean(params.brand_uuid),
    placeholderData: keepPreviousData,
  });
}

/** body_type / powertrain facet options of one brand's models (TEC-369). */
export function useVehicleModelFacets(brandUuid: string) {
  return useQuery({
    queryKey: vehicleCatalogKeys.modelFacets(brandUuid),
    queryFn: () => vehicleCatalogService.modelFacets({ brand_uuid: brandUuid }),
    enabled: Boolean(brandUuid),
    staleTime: 60_000,
  });
}

function useInvalidate() {
  const queryClient = useQueryClient();
  return {
    queryClient,
    brand: (brand?: VehicleBrand) => {
      if (brand) {
        queryClient.setQueryData(vehicleCatalogKeys.brand(brand.uuid), brand);
      }
      void queryClient.invalidateQueries({
        queryKey: vehicleCatalogKeys.brandLists(),
      });
    },
    // Model lists and the brand (model_count changes with every model write).
    models: () =>
      queryClient.invalidateQueries({ queryKey: vehicleCatalogKeys.all }),
  };
}

export function useVehicleBrandMutations() {
  const { t } = useLocale();
  const invalidate = useInvalidate();

  return {
    create: useAppMutation({
      mutationFn: (body: VehicleBrandInput) =>
        vehicleCatalogService.createBrand(body),
      onSuccess: (brand) => {
        invalidate.brand(brand);
        appToast.success(t("vehicles.toast.brand_created"));
      },
    }),
    update: useAppMutation({
      mutationFn: ({ uuid, body }: { uuid: string; body: VehicleBrandInput }) =>
        vehicleCatalogService.updateBrand(uuid, body),
      onSuccess: (brand) => {
        invalidate.brand(brand);
        appToast.success(t("vehicles.toast.brand_updated"));
      },
    }),
    remove: useAppMutation({
      mutationFn: (uuid: string) => vehicleCatalogService.deleteBrand(uuid),
      onSuccess: (_data, uuid) => {
        invalidate.queryClient.removeQueries({
          queryKey: vehicleCatalogKeys.brand(uuid),
        });
        invalidate.brand();
        appToast.success(t("vehicles.toast.brand_deleted"));
      },
    }),
    uploadImage: useAppMutation({
      mutationFn: ({
        uuid,
        kind,
        file,
      }: {
        uuid: string;
        kind: "logo" | "hero";
        file: File;
      }) => vehicleCatalogService.uploadBrandImage(uuid, kind, file),
      onSuccess: (brand, { kind }) => {
        invalidate.brand(brand);
        appToast.success(
          t(
            kind === "logo"
              ? "vehicles.toast.logo_uploaded"
              : "vehicles.toast.hero_uploaded",
          ),
        );
      },
    }),
    deleteImage: useAppMutation({
      mutationFn: ({ uuid, kind }: { uuid: string; kind: "logo" | "hero" }) =>
        vehicleCatalogService.deleteBrandImage(uuid, kind),
      onSuccess: (brand) => {
        invalidate.brand(brand);
        appToast.success(t("vehicles.toast.image_removed"));
      },
    }),
  };
}

export function useVehicleModelMutations() {
  const { t } = useLocale();
  const invalidate = useInvalidate();

  return {
    create: useAppMutation({
      mutationFn: (body: VehicleModelInput) =>
        vehicleCatalogService.createModel(body),
      onSuccess: () => {
        void invalidate.models();
        appToast.success(t("vehicles.toast.model_created"));
      },
    }),
    update: useAppMutation({
      mutationFn: ({ uuid, body }: { uuid: string; body: VehicleModelInput }) =>
        vehicleCatalogService.updateModel(uuid, body),
      onSuccess: () => {
        void invalidate.models();
        appToast.success(t("vehicles.toast.model_updated"));
      },
    }),
    remove: useAppMutation({
      mutationFn: (uuid: string) => vehicleCatalogService.deleteModel(uuid),
      onSuccess: () => {
        void invalidate.models();
        appToast.success(t("vehicles.toast.model_deleted"));
      },
    }),
    uploadHero: useAppMutation({
      mutationFn: ({ uuid, file }: { uuid: string; file: File }) =>
        vehicleCatalogService.uploadModelHero(uuid, file),
      onSuccess: () => {
        void invalidate.models();
        appToast.success(t("vehicles.toast.hero_uploaded"));
      },
    }),
    deleteHero: useAppMutation({
      mutationFn: (uuid: string) => vehicleCatalogService.deleteModelHero(uuid),
      onSuccess: () => {
        void invalidate.models();
        appToast.success(t("vehicles.toast.image_removed"));
      },
    }),
  };
}
