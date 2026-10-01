"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  documentTemplatesService,
  type DocumentTemplate,
  type DocumentTemplateSaveRequest,
  type ListDocumentTemplatesParams,
} from "@/features/document-templates/services/document-templates.service";
import { ApiError } from "@/lib/api/errors";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const documentTemplatesKeys = {
  all: ["platform", "document-templates"] as const,
  kinds: () => [...documentTemplatesKeys.all, "kinds"] as const,
  lists: () => [...documentTemplatesKeys.all, "list"] as const,
  list: (params: ListDocumentTemplatesParams) =>
    [...documentTemplatesKeys.lists(), params] as const,
  detail: (uuid: string) =>
    [...documentTemplatesKeys.all, "detail", uuid] as const,
  versions: (uuid: string) =>
    [...documentTemplatesKeys.all, "versions", uuid] as const,
};

export function useDocumentKinds() {
  return useQuery({
    queryKey: documentTemplatesKeys.kinds(),
    queryFn: () => documentTemplatesService.kinds(),
    staleTime: 10 * 60_000,
  });
}

export function useDocumentTemplates(
  params: ListDocumentTemplatesParams,
  enabled = true,
) {
  return useQuery({
    queryKey: documentTemplatesKeys.list(params),
    queryFn: () => documentTemplatesService.list(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useDocumentTemplate(uuid: string | null) {
  return useQuery({
    queryKey: documentTemplatesKeys.detail(uuid ?? ""),
    queryFn: () => documentTemplatesService.get(uuid!),
    enabled: Boolean(uuid),
  });
}

export function useDocumentTemplateVersions(uuid: string | null) {
  return useQuery({
    queryKey: documentTemplatesKeys.versions(uuid ?? ""),
    queryFn: () => documentTemplatesService.versions(uuid!),
    enabled: Boolean(uuid),
  });
}

function errorMessage(err: unknown, fallback: string) {
  if (err instanceof ApiError) {
    const unknown = err.details
      .map((d) => d.message)
      .filter(Boolean)
      .join(", ");
    return unknown || err.message || fallback;
  }
  return fallback;
}

function useRefresh() {
  const queryClient = useQueryClient();
  return (row: DocumentTemplate) => {
    queryClient.setQueryData(documentTemplatesKeys.detail(row.uuid), row);
    void queryClient.invalidateQueries({
      queryKey: documentTemplatesKeys.lists(),
    });
    void queryClient.invalidateQueries({
      queryKey: [...documentTemplatesKeys.all, "versions"],
    });
  };
}

export function useSaveDocumentTemplateDraft() {
  const { t } = useLocale();
  const refresh = useRefresh();
  return useAppMutation({
    mutationFn: (body: DocumentTemplateSaveRequest) =>
      documentTemplatesService.saveDraft(body),
    onSuccess: (row) => {
      refresh(row);
      appToast.success(t("documents.toast.draft_saved"));
    },
    onError: (err) =>
      appToast.error(errorMessage(err, t("documents.toast.save_failed"))),
  });
}

export function usePublishDocumentTemplate() {
  const { t } = useLocale();
  const refresh = useRefresh();
  return useAppMutation({
    mutationFn: (uuid: string) => documentTemplatesService.publish(uuid),
    onSuccess: (row) => {
      refresh(row);
      appToast.success(t("documents.toast.published"));
    },
    onError: (err) =>
      appToast.error(errorMessage(err, t("documents.toast.publish_failed"))),
  });
}
