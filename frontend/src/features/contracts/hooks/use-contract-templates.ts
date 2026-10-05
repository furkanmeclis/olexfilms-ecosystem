"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  contractTemplatesService,
  type ContractTemplate,
  type ContractTemplateLocaleRequest,
  type ContractTemplateRequest,
} from "@/features/contracts/services/contract-templates.service";
import { ApiError } from "@/lib/api/errors";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const contractTemplatesKeys = {
  all: ["platform", "contract-templates"] as const,
  list: () => [...contractTemplatesKeys.all, "list"] as const,
  detail: (uuid: string) =>
    [...contractTemplatesKeys.all, "detail", uuid] as const,
  variables: () => [...contractTemplatesKeys.all, "variables"] as const,
};

export function useContractTemplates() {
  return useQuery({
    queryKey: contractTemplatesKeys.list(),
    queryFn: () => contractTemplatesService.list(),
  });
}

export function useContractTemplate(uuid: string) {
  return useQuery({
    queryKey: contractTemplatesKeys.detail(uuid),
    queryFn: () => contractTemplatesService.get(uuid),
    enabled: Boolean(uuid),
  });
}

export function useContractTemplateVariables() {
  return useQuery({
    queryKey: contractTemplatesKeys.variables(),
    queryFn: () => contractTemplatesService.variables(),
    staleTime: 10 * 60_000,
  });
}

function errorMessage(err: unknown, fallback: string) {
  if (err instanceof ApiError) {
    const details = err.details
      .map((d) => d.message)
      .filter(Boolean)
      .join(", ");
    return details || err.message || fallback;
  }
  return fallback;
}

/** Writes the default flag into the cached list (one default per kind). */
export function applyDefault(
  items: readonly ContractTemplate[],
  row: ContractTemplate,
): ContractTemplate[] {
  return items.map((item) => {
    if (item.uuid === row.uuid)
      return { ...item, ...row, locales: item.locales };
    if (item.kind === row.kind && item.is_default) {
      return { ...item, is_default: false };
    }
    return item;
  });
}

export function useCreateContractTemplate() {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (body: ContractTemplateRequest) =>
      contractTemplatesService.create(body),
    onSuccess: (row) => {
      queryClient.setQueryData(contractTemplatesKeys.detail(row.uuid), row);
      void queryClient.invalidateQueries({
        queryKey: contractTemplatesKeys.list(),
      });
      appToast.success(t("contract_templates.toast.created"));
    },
    onError: (err) =>
      appToast.error(
        errorMessage(err, t("contract_templates.toast.save_failed")),
      ),
  });
}

export function useUpdateContractTemplate(uuid: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (body: ContractTemplateRequest) =>
      contractTemplatesService.update(uuid, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: contractTemplatesKeys.all,
      });
      appToast.success(t("contract_templates.toast.saved"));
    },
    onError: (err) =>
      appToast.error(
        errorMessage(err, t("contract_templates.toast.save_failed")),
      ),
  });
}

export function usePutContractTemplateLocale(uuid: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (vars: {
      locale: string;
      body: ContractTemplateLocaleRequest;
    }) => contractTemplatesService.putLocale(uuid, vars.locale, vars.body),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: contractTemplatesKeys.detail(uuid),
      });
      appToast.success(t("contract_templates.toast.locale_saved"));
    },
    onError: (err) =>
      appToast.error(
        errorMessage(err, t("contract_templates.toast.save_failed")),
      ),
  });
}

export function useSetDefaultContractTemplate() {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (uuid: string) => contractTemplatesService.setDefault(uuid),
    onSuccess: (row) => {
      queryClient.setQueryData<{ items: ContractTemplate[] }>(
        contractTemplatesKeys.list(),
        (prev) => (prev ? { items: applyDefault(prev.items, row) } : prev),
      );
      void queryClient.invalidateQueries({
        queryKey: contractTemplatesKeys.all,
      });
      appToast.success(t("contract_templates.toast.default_set"));
    },
    onError: (err) =>
      appToast.error(
        errorMessage(err, t("contract_templates.toast.default_failed")),
      ),
  });
}
