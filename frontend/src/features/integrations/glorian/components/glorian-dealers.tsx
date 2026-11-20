"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Link2, Unlink } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { FormProvider, useForm, useWatch } from "react-hook-form";
import { z } from "zod";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import {
  AppCombobox,
  AppRadioGroup,
  type ComboboxOption,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FieldGroup } from "@/components/ui/field";
import { glorianErrorText } from "@/features/integrations/glorian/lib/errors";
import { glorianKeys } from "@/features/integrations/glorian/lib/keys";
import {
  glorianService,
  type GlorianDealer,
  type GlorianDealerFilter,
  type GlorianDealerMatchType,
  type GlorianDealerOrganization,
} from "@/features/integrations/glorian/services/glorian.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useFormatter, useLocale } from "@/providers/locale-provider";
import { appToast } from "@/lib/toast";

export const DEALERS_PERSIST_KEY = "platform-glorian-dealers-v1";

export const DEALER_MATCH_TYPES: GlorianDealerMatchType[] = [
  "manual",
  "code",
  "phone",
  "ambiguous",
  "none",
];

type Variant = "success" | "warning" | "danger" | "secondary" | "outline";

const MATCH_VARIANT: Record<GlorianDealerMatchType, Variant> = {
  manual: "success",
  code: "success",
  phone: "secondary",
  ambiguous: "danger",
  none: "warning",
};

/** Orders of these dealers' organizations are held until fixed. */
export function needsAttention(
  d: Pick<GlorianDealer, "match_type" | "active">,
) {
  return d.active && (d.match_type === "ambiguous" || d.match_type === "none");
}

/** "Name · City" of an organization option. */
export function organizationLabel(
  o: Pick<GlorianDealerOrganization, "name" | "city">,
) {
  return o.city ? `${o.name} · ${o.city}` : o.name;
}

function organizationOption(o: GlorianDealerOrganization): ComboboxOption {
  const extra = [o.glorian_dealer_code, o.phone].filter(Boolean).join(" · ");
  return {
    value: o.uuid,
    label: organizationLabel(o),
    description: extra || undefined,
  };
}

/** The list opened on the dealers that need a decision. */
const DEFAULT_MATCH_FILTER = [
  { id: "match_type", value: ["ambiguous", "none"] },
];

/**
 * Glorian dealer ↔ organization match (TEC-1265, decision 6): every hub
 * dealer with the organization its orders go to and how it was found
 * (manual link, Glorian dealer code, phone fallback). Admins fix a missing
 * or ambiguous match by writing the dealer code onto the organization or
 * by linking it by hand.
 */
export function GlorianDealers({ canManage }: { canManage: boolean }) {
  const { t } = useLocale();
  const format = useFormatter();
  const queryClient = useQueryClient();
  const { confirm } = useDialogs();
  const [linking, setLinking] = useState<GlorianDealer | null>(null);

  const unlink = useMutation({
    mutationFn: (uuid: string) => glorianService.linkDealer(uuid, null),
    onSuccess: async () => {
      appToast.success(t("integrations.glorian.dealers.unlinked"));
      await queryClient.invalidateQueries({
        queryKey: [...glorianKeys.all, "dealers"],
      });
    },
    onError: (error) => appToast.error(glorianErrorText(t, error)),
  });
  const unlinkDealer = unlink.mutate;

  const columns = useMemo<ColumnDef<GlorianDealer, unknown>[]>(
    () => [
      createColumn<GlorianDealer>({
        accessorKey: "name",
        labelKey: "integrations.glorian.dealers.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="truncate font-medium">{row.original.name}</div>
            {!row.original.active ? (
              <div className="text-muted-foreground text-xs">
                {t("integrations.glorian.dealers.inactive")}
              </div>
            ) : null}
          </div>
        ),
      }),
      createColumn<GlorianDealer>({
        accessorKey: "dealer_code",
        labelKey: "integrations.glorian.dealers.code",
        enableSorting: true,
        cell: ({ row }) =>
          row.original.dealer_code ? (
            <span className="font-mono">{row.original.dealer_code}</span>
          ) : (
            "—"
          ),
      }),
      createColumn<GlorianDealer>({
        accessorKey: "phone",
        labelKey: "integrations.glorian.dealers.phone",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="whitespace-nowrap" dir="ltr">
            {row.original.phone ?? "—"}
          </span>
        ),
      }),
      createColumn<GlorianDealer>({
        accessorKey: "match_type",
        labelKey: "integrations.glorian.dealers.match",
        enableSorting: true,
        filterVariant: "faceted",
        param: "match_type",
        gridSecondary: true,
        filterOptions: DEALER_MATCH_TYPES.map((value) => ({
          value,
          labelKey: `integrations.glorian.dealer_match.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <Badge
            variant={MATCH_VARIANT[row.original.match_type] ?? "outline"}
            title={t(
              `integrations.glorian.dealer_match_hint.${row.original.match_type}`,
            )}
          >
            {t(`integrations.glorian.dealer_match.${row.original.match_type}`)}
          </Badge>
        ),
      }),
      createColumn<GlorianDealer>({
        id: "organization",
        accessorFn: (row) =>
          row.organization ? organizationLabel(row.organization) : "",
        labelKey: "integrations.glorian.dealers.organization",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.organization
            ? organizationLabel(row.original.organization)
            : "—",
      }),
      createColumn<GlorianDealer>({
        accessorKey: "active",
        labelKey: "integrations.glorian.dealers.active",
        enableSorting: false,
        filterVariant: "boolean",
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.active ? t("common.yes") : t("common.no"),
      }),
      createColumn<GlorianDealer>({
        accessorKey: "synced_at",
        labelKey: "integrations.glorian.dealers.synced",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {row.original.synced_at
              ? format.dateTime(row.original.synced_at)
              : "—"}
          </span>
        ),
      }),
      ...(canManage
        ? [
            createColumn<GlorianDealer>({
              id: "actions",
              labelKey: "common.actions",
              enableSorting: false,
              enableHiding: false,
              enableResizing: false,
              cell: ({ row }) => {
                const dealer = row.original;
                const actions: EntityRowAction[] = [
                  {
                    id: "link",
                    label: t("integrations.glorian.dealers.link"),
                    icon: Link2,
                    onSelect: () => setLinking(dealer),
                  },
                ];
                if (dealer.match_type === "manual") {
                  actions.push({
                    id: "unlink",
                    label: t("integrations.glorian.dealers.unlink"),
                    icon: Unlink,
                    variant: "destructive",
                    onSelect: () => {
                      void (async () => {
                        const ok = await confirm({
                          title: t("integrations.glorian.dealers.unlink"),
                          description: t(
                            "integrations.glorian.dealers.unlink_confirm",
                            { name: dealer.name },
                          ),
                        });
                        if (ok) unlinkDealer(dealer.uuid);
                      })();
                    },
                  });
                }
                return <EntityRowActions actions={actions} />;
              },
            }),
          ]
        : []),
    ],
    [canManage, confirm, format, t, unlinkDealer],
  );

  const listState = useServerListState({
    columns,
    initialSort: "name",
    initialPageSize: 20,
    persistKey: DEALERS_PERSIST_KEY,
    initialColumnFilters: DEFAULT_MATCH_FILTER,
  });
  const query = listState.params as GlorianDealerFilter;

  const list = useQuery({
    queryKey: glorianKeys.dealers(query),
    queryFn: () => glorianService.dealersPage(query),
    placeholderData: (previous) => previous,
  });

  return (
    <div data-testid="dealers">
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={glorianErrorText(t, list.error)}
        onRetry={() => void list.refetch()}
        emptyTitle={t("integrations.glorian.dealers.empty")}
        emptyDescription={t("integrations.glorian.dealers.empty_hint")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: DEALERS_PERSIST_KEY }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
      {canManage ? (
        <GlorianDealerLinkDialog
          dealer={linking}
          onOpenChange={(open) => {
            if (!open) setLinking(null);
          }}
        />
      ) : null}
    </div>
  );
}

type LinkMode = "code" | "manual";

type LinkFormValues = { organization_uuid: string; mode: LinkMode };

/**
 * Link dialog of one dealer. A dealer with a code is matched by writing
 * the code onto the organization (the lasting rule of decision 6); one
 * without a code, or an organization that must keep another code, is
 * linked by hand.
 */
export function GlorianDealerLinkDialog({
  dealer,
  onOpenChange,
}: {
  dealer: GlorianDealer | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  return (
    <Dialog open={Boolean(dealer)} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t("integrations.glorian.dealers.link_title", {
              name: dealer?.name ?? "",
            })}
          </DialogTitle>
          <DialogDescription>
            {t("integrations.glorian.dealers.link_hint")}
          </DialogDescription>
        </DialogHeader>
        {dealer ? (
          <DealerLinkForm
            key={dealer.uuid}
            dealer={dealer}
            onDone={() => onOpenChange(false)}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function DealerLinkForm({
  dealer,
  onDone,
}: {
  dealer: GlorianDealer;
  onDone: () => void;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const hasCode = Boolean(dealer.dealer_code);

  const schema = useMemo(
    () =>
      z.object({
        organization_uuid: z
          .string()
          .min(1, t("integrations.glorian.validation.required")),
        mode: z.enum(["code", "manual"]),
      }),
    [t],
  );
  const form = useForm<LinkFormValues>({
    resolver: zodResolver(schema as never),
    defaultValues: {
      organization_uuid: dealer.organization?.uuid ?? "",
      mode: hasCode ? "code" : "manual",
    },
  });
  const mode = useWatch({ control: form.control, name: "mode" });

  const loadOrganizations = useCallback(async (q: string) => {
    const items = await glorianService.dealerOrganizations(q);
    return items.map(organizationOption);
  }, []);
  const initialOptions = useMemo(
    () =>
      dealer.organization ? [organizationOption(dealer.organization)] : [],
    [dealer.organization],
  );

  const save = useMutation({
    mutationFn: async (values: LinkFormValues) => {
      if (values.mode === "code" && dealer.dealer_code) {
        await glorianService.setDealerCode(
          values.organization_uuid,
          dealer.dealer_code,
        );
        // A manual link beats the code: clear it so the code applies.
        if (dealer.match_type === "manual") {
          await glorianService.linkDealer(dealer.uuid, null);
        }
        return;
      }
      await glorianService.linkDealer(dealer.uuid, values.organization_uuid);
    },
    onSuccess: async () => {
      appToast.success(t("integrations.glorian.dealers.saved"));
      await queryClient.invalidateQueries({
        queryKey: [...glorianKeys.all, "dealers"],
      });
      onDone();
    },
    onError: (error) => appToast.error(glorianErrorText(t, error)),
  });

  return (
    <FormProvider {...form}>
      <form
        noValidate
        onSubmit={form.handleSubmit((values) => save.mutate(values))}
        data-testid="dealer-link-form"
      >
        <FieldGroup className="gap-4">
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
            <dt className="text-muted-foreground">
              {t("integrations.glorian.dealers.code")}
            </dt>
            <dd className="font-mono">{dealer.dealer_code ?? "—"}</dd>
            <dt className="text-muted-foreground">
              {t("integrations.glorian.dealers.phone")}
            </dt>
            <dd dir="ltr" className="text-start">
              {dealer.phone ?? "—"}
            </dd>
          </dl>
          <AppCombobox
            name="organization_uuid"
            label={t("integrations.glorian.dealers.organization")}
            loadOptions={loadOrganizations}
            initialOptions={initialOptions}
            placeholder={t(
              "integrations.glorian.dealers.organization_placeholder",
            )}
            emptyText={t("integrations.glorian.dealers.no_organizations")}
            data-testid="dealer-link-organization"
          />
          {hasCode ? (
            <AppRadioGroup
              name="mode"
              label={t("integrations.glorian.dealers.mode")}
              options={[
                {
                  value: "code",
                  label: t("integrations.glorian.dealers.mode_code", {
                    code: dealer.dealer_code ?? "",
                  }),
                  description: t("integrations.glorian.dealers.mode_code_hint"),
                },
                {
                  value: "manual",
                  label: t("integrations.glorian.dealers.mode_manual"),
                  description: t(
                    "integrations.glorian.dealers.mode_manual_hint",
                  ),
                },
              ]}
            />
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("integrations.glorian.dealers.mode_manual_hint")}
            </p>
          )}
          <p className="text-muted-foreground text-xs">
            {t("integrations.glorian.dealers.replay_hint")}
          </p>
        </FieldGroup>
        <DialogFooter className="mt-6">
          <Button type="button" variant="outline" onClick={onDone}>
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            disabled={save.isPending}
            data-testid="dealer-link-save"
          >
            {save.isPending
              ? t("common.saving")
              : mode === "code"
                ? t("integrations.glorian.dealers.save_code")
                : t("integrations.glorian.dealers.save_manual")}
          </Button>
        </DialogFooter>
      </form>
    </FormProvider>
  );
}
