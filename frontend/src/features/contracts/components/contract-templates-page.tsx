"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useContractTemplates,
  useCreateContractTemplate,
  useSetDefaultContractTemplate,
} from "@/features/contracts/hooks/use-contract-templates";
import {
  CONTRACT_TEMPLATE_KINDS,
  type ContractTemplate,
  type ContractTemplateKind,
} from "@/features/contracts/services/contract-templates.service";
import { ApiError } from "@/lib/api/errors";
import { useLocale } from "@/providers/locale-provider";

/** Contract template list: kind, default badge, active, "make default". */
export function ContractTemplatesPage() {
  const { t } = useLocale();
  const query = useContractTemplates();
  const setDefault = useSetDefaultContractTemplate();
  const [pending, setPending] = useState<ContractTemplate | null>(null);
  const [creating, setCreating] = useState(false);
  const items = query.data?.items ?? [];
  const moduleOff =
    query.error instanceof ApiError && query.error.code === "FEATURE_DISABLED";

  return (
    <EntityPage
      title={t("contract_templates.title")}
      description={t("contract_templates.description")}
      permission={permissions.contractTemplates.manage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("contract_templates.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("contract_templates.title") },
      ]}
      actions={
        <Button onClick={() => setCreating(true)} disabled={moduleOff}>
          {t("contract_templates.actions.new")}
        </Button>
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={
            moduleOff
              ? t("contract_templates.module_disabled")
              : t("contract_templates.load_failed")
          }
        />
      ) : null}

      {query.data ? (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="text-muted-foreground text-xs">
              <tr>
                <th className="px-4 py-2 text-start font-medium">
                  {t("contract_templates.fields.name")}
                </th>
                <th className="px-4 py-2 text-start font-medium">
                  {t("contract_templates.fields.kind")}
                </th>
                <th className="px-4 py-2 text-start font-medium">
                  {t("contract_templates.fields.status")}
                </th>
                <th className="px-4 py-2" />
              </tr>
            </thead>
            <tbody>
              {items.length === 0 ? (
                <tr className="border-t">
                  <td
                    colSpan={4}
                    className="text-muted-foreground px-4 py-6 text-center"
                  >
                    {t("contract_templates.empty")}
                  </td>
                </tr>
              ) : null}
              {items.map((row) => (
                <tr
                  key={row.uuid}
                  className="border-t"
                  data-testid={`contract-template-${row.uuid}`}
                >
                  <td className="px-4 py-2">
                    <span className="flex flex-wrap items-center gap-2">
                      {row.name}
                      {row.is_default ? (
                        <Badge variant="success" data-testid="default-badge">
                          {t("contract_templates.default_badge")}
                        </Badge>
                      ) : null}
                    </span>
                  </td>
                  <td className="px-4 py-2">
                    {t(`contract_templates.kinds.${row.kind}`)}
                  </td>
                  <td className="px-4 py-2">
                    <Badge variant={row.is_active ? "secondary" : "outline"}>
                      {row.is_active
                        ? t("contract_templates.status.active")
                        : t("contract_templates.status.inactive")}
                    </Badge>
                  </td>
                  <td className="px-4 py-2">
                    <span className="flex flex-wrap justify-end gap-2">
                      {!row.is_default && row.is_active ? (
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setPending(row)}
                        >
                          {t("contract_templates.actions.make_default")}
                        </Button>
                      ) : null}
                      <Button asChild size="sm" variant="outline">
                        <Link
                          href={routes.platform.contractTemplates.edit(
                            row.uuid,
                          )}
                        >
                          {t("contract_templates.actions.edit")}
                        </Link>
                      </Button>
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      <ConfirmDialog
        open={pending !== null}
        title={t("contract_templates.default_confirm.title")}
        description={
          pending
            ? t("contract_templates.default_confirm.description", {
                name: pending.name,
                kind: t(`contract_templates.kinds.${pending.kind}`),
              })
            : undefined
        }
        confirmLabel={t("contract_templates.actions.make_default")}
        isPending={setDefault.isPending}
        onCancel={() => setPending(null)}
        onConfirm={() => {
          if (!pending) return;
          setDefault.mutate(pending.uuid, {
            onSettled: () => setPending(null),
          });
        }}
      />

      {creating ? (
        <CreateTemplateDialog onClose={() => setCreating(false)} />
      ) : null}
    </EntityPage>
  );
}

function CreateTemplateDialog({ onClose }: { onClose: () => void }) {
  const { t } = useLocale();
  const router = useRouter();
  const create = useCreateContractTemplate();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<ContractTemplateKind>("vehicle_intake");

  const submit = async () => {
    const row = await create.mutateAsync({
      name: name.trim(),
      kind,
      is_active: true,
    });
    onClose();
    router.push(routes.platform.contractTemplates.edit(row.uuid));
  };

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !create.isPending) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("contract_templates.create.title")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="contract-template-name">
              {t("contract_templates.fields.name")}
            </Label>
            <Input
              id="contract-template-name"
              value={name}
              maxLength={150}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label>{t("contract_templates.fields.kind")}</Label>
            <Select
              value={kind}
              onValueChange={(v) => setKind(v as ContractTemplateKind)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CONTRACT_TEMPLATE_KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`contract_templates.kinds.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={onClose}
            disabled={create.isPending}
          >
            {t("common.cancel")}
          </Button>
          <Button
            onClick={() => void submit().catch(() => undefined)}
            disabled={create.isPending || !name.trim()}
          >
            {t("contract_templates.actions.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
