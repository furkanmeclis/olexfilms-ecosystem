"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { CheckCircle2, FileText, Plus, RotateCcw, XCircle } from "lucide-react";
import { useMemo, useState } from "react";

import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import {
  CertificateStatusChip,
  WarningDecisionChip,
} from "@/features/certificates/components/certificate-status";
import {
  CERTIFICATE_STATUSES,
  WARNING_DECISIONS,
  certificateKeys,
  certificatesService,
  localizedName,
  personName,
  validateCertificatePDF,
  validateRejectReason,
  type Certificate,
  type CertificateType,
  type CertificateWarning,
} from "@/features/certificates/services/certificates.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const TYPE_KEY = "platform-certificate-types-v1";
const CERT_KEY = "tenant-certificates-v1";
const VERIFY_KEY = "tenant-certificate-verification-v1";
const APPROVAL_KEY = "tenant-certificate-approvals-v1";

function invalidateAll(queryClient: ReturnType<typeof useQueryClient>) {
  return queryClient.invalidateQueries({ queryKey: certificateKeys.all });
}

function toastError(error: unknown, fallback: string) {
  appToast.error(isApiError(error) ? error.message : fallback);
}

function TypeDialog({
  open,
  type,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  type: CertificateType | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: {
    nameTr: string;
    nameEn: string;
    validity: string;
    active: boolean;
    categories: string;
    products: string;
  }) => void;
}) {
  const { t } = useLocale();
  const [nameTr, setNameTr] = useState("");
  const [nameEn, setNameEn] = useState("");
  const [validity, setValidity] = useState("");
  const [active, setActive] = useState(true);
  const [categories, setCategories] = useState("");
  const [products, setProducts] = useState("");

  function openChanged(next: boolean) {
    if (next) {
      setNameTr(localizedName(type?.name, "tr"));
      setNameEn(localizedName(type?.name, "en"));
      setValidity(type?.validity_months ? String(type.validity_months) : "");
      setActive(type?.active ?? true);
      setCategories("");
      setProducts("");
    }
    onOpenChange(next);
  }

  return (
    <Dialog open={open} onOpenChange={openChanged}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {type ? t("certificates.types.edit") : t("certificates.types.new")}
          </DialogTitle>
          <DialogDescription>
            {t("certificates.types.form_description")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="cert-name-tr">
                {t("certificates.fields.name_tr")}
              </Label>
              <Input
                id="cert-name-tr"
                value={nameTr}
                onChange={(e) => setNameTr(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="cert-name-en">
                {t("certificates.fields.name_en")}
              </Label>
              <Input
                id="cert-name-en"
                value={nameEn}
                onChange={(e) => setNameEn(e.target.value)}
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="cert-validity">
              {t("certificates.fields.validity_months")}
            </Label>
            <Input
              id="cert-validity"
              inputMode="numeric"
              value={validity}
              onChange={(e) => setValidity(e.target.value)}
            />
          </div>
          <div className="grid gap-2 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="cert-cats">
                {t("certificates.fields.category_uuids")}
              </Label>
              <Textarea
                id="cert-cats"
                value={categories}
                onChange={(e) => setCategories(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="cert-products">
                {t("certificates.fields.product_uuids")}
              </Label>
              <Textarea
                id="cert-products"
                value={products}
                onChange={(e) => setProducts(e.target.value)}
              />
            </div>
          </div>
          <label className="flex items-center gap-2 text-sm">
            <Switch checked={active} onCheckedChange={setActive} />
            {t("certificates.fields.active")}
          </label>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            onClick={() =>
              onSubmit({
                nameTr,
                nameEn,
                validity,
                active,
                categories,
                products,
              })
            }
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function CertificateTypesPage() {
  const { t, locale } = useLocale();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<CertificateType | null>(null);
  const [open, setOpen] = useState(false);

  const columns = useMemo(
    () =>
      [
        createColumn<CertificateType>({
          accessorKey: "name",
          labelKey: "certificates.fields.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => localizedName(row.original.name, locale),
        }),
        createColumn<CertificateType>({
          accessorKey: "validity_months",
          labelKey: "certificates.fields.validity_months",
          enableSorting: true,
          cell: ({ row }) => row.original.validity_months ?? "—",
        }),
        createColumn<CertificateType>({
          id: "category_count",
          accessorFn: (row) => row.category_count ?? 0,
          labelKey: "certificates.fields.category_count",
          enableSorting: false,
        }),
        createColumn<CertificateType>({
          id: "product_count",
          accessorFn: (row) => row.product_count ?? 0,
          labelKey: "certificates.fields.product_count",
          enableSorting: false,
        }),
        createColumn<CertificateType>({
          accessorKey: "active",
          labelKey: "certificates.fields.active",
          enableSorting: true,
          filterVariant: "boolean",
          cell: ({ row }) =>
            row.original.active ? t("common.yes") : t("common.no"),
        }),
        createColumn<CertificateType>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t("common.edit"),
                  icon: FileText,
                  onSelect: () => {
                    setEditing(row.original);
                    setOpen(true);
                  },
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<CertificateType, unknown>[],
    [locale, t],
  );
  const list = useServerListState({
    columns,
    initialSort: "sort_order",
    persistKey: TYPE_KEY,
  });
  const query = useQuery({
    queryKey: certificateKeys.types(list.params),
    queryFn: () => certificatesService.listTypes(list.params),
  });
  const save = useMutation({
    mutationFn: (
      values: Parameters<typeof TypeDialog>[0]["onSubmit"] extends (
        v: infer V,
      ) => void
        ? V
        : never,
    ) => {
      const csv = (text: string) =>
        text
          .split(/[,\s]+/)
          .map((v) => v.trim())
          .filter(Boolean);
      const body = {
        name: {
          tr: values.nameTr.trim(),
          en: values.nameEn.trim() || values.nameTr.trim(),
        },
        validity_months: values.validity.trim()
          ? Number(values.validity)
          : null,
        active: values.active,
        sort_order: editing?.sort_order ?? 1000,
        category_uuids: csv(values.categories),
        product_uuids: csv(values.products),
      };
      return editing
        ? certificatesService.updateType(editing.uuid, body)
        : certificatesService.createType(body);
    },
    onSuccess: async () => {
      await invalidateAll(queryClient);
      appToast.success(t("certificates.toast.saved"));
      setOpen(false);
      setEditing(null);
    },
    onError: (err) => toastError(err, t("certificates.toast.failed")),
  });

  return (
    <EntityPage
      title={t("certificates.types.title")}
      description={t("certificates.types.description")}
      permission={permissions.certificates.typesManage}
      actions={
        <Button
          onClick={() => {
            setEditing(null);
            setOpen(true);
          }}
        >
          <Plus className="size-4" />
          {t("certificates.types.new")}
        </Button>
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        rowCount={query.data?.total ?? 0}
        state={list.tableState}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        features={{ persistKey: TYPE_KEY, rowSelection: true }}
      />
      <TypeDialog
        open={open}
        type={editing}
        onOpenChange={setOpen}
        onSubmit={(v) => save.mutate(v)}
      />
    </EntityPage>
  );
}

function UploadDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [userUuid, setUserUuid] = useState("");
  const [typeUuid, setTypeUuid] = useState("");
  const [issuedAt, setIssuedAt] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState("");
  const upload = useMutation({
    mutationFn: () => {
      const validation = validateCertificatePDF(file);
      if (validation) {
        throw new Error(t(`certificates.upload.${validation}`));
      }
      return certificatesService.uploadCertificate({
        userUuid,
        typeUuid,
        issuedAt,
        file: file!,
      });
    },
    onSuccess: async () => {
      await invalidateAll(queryClient);
      appToast.success(t("certificates.toast.uploaded"));
      onOpenChange(false);
    },
    onError: (err) => {
      const message =
        err instanceof Error ? err.message : t("certificates.toast.failed");
      setError(message);
      appToast.error(message);
    },
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("certificates.upload.title")}</DialogTitle>
          <DialogDescription>
            {t("certificates.upload.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <Input
            aria-label={t("certificates.fields.staff_uuid")}
            placeholder={t("certificates.fields.staff_uuid")}
            value={userUuid}
            onChange={(e) => setUserUuid(e.target.value)}
          />
          <Input
            aria-label={t("certificates.fields.type_uuid")}
            placeholder={t("certificates.fields.type_uuid")}
            value={typeUuid}
            onChange={(e) => setTypeUuid(e.target.value)}
          />
          <Input
            inputMode="numeric"
            placeholder="2026-10-08"
            value={issuedAt}
            onChange={(e) => setIssuedAt(e.target.value)}
          />
          <Input
            type="file"
            accept="application/pdf,.pdf"
            onChange={(e) => {
              setError("");
              setFile(e.target.files?.[0] ?? null);
            }}
          />
          {error ? (
            <p
              className="text-destructive text-sm"
              data-testid="certificate-upload-error"
            >
              {error}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button onClick={() => upload.mutate()} disabled={upload.isPending}>
            {t("certificates.upload.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DecisionDialog({
  title,
  open,
  requireReason,
  onOpenChange,
  onSubmit,
}: {
  title: string;
  open: boolean;
  requireReason: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (reason: string) => void;
}) {
  const { t } = useLocale();
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <Textarea
          value={reason}
          onChange={(e) => {
            setError("");
            setReason(e.target.value);
          }}
          placeholder={t("certificates.decisions.reason")}
        />
        {error ? (
          <p
            className="text-destructive text-sm"
            data-testid="certificate-decision-error"
          >
            {error}
          </p>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={() => {
              if (requireReason && !validateRejectReason(reason)) {
                setError(t("certificates.decisions.reason_required"));
                return;
              }
              onSubmit(reason.trim());
            }}
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function CertificatesPage({ queue = false }: { queue?: boolean }) {
  const { t, format, locale } = useLocale();
  const queryClient = useQueryClient();
  const [uploadOpen, setUploadOpen] = useState(false);
  const [rejecting, setRejecting] = useState<Certificate | null>(null);
  const verify = useMutation({
    mutationFn: certificatesService.verify,
    onSuccess: async () => invalidateAll(queryClient),
    onError: (e) => toastError(e, t("certificates.toast.failed")),
  });
  const revoke = useMutation({
    mutationFn: certificatesService.revoke,
    onSuccess: async () => invalidateAll(queryClient),
    onError: (e) => toastError(e, t("certificates.toast.failed")),
  });
  const columns = useMemo(
    () =>
      [
        createColumn<Certificate>({
          id: "user_name",
          accessorFn: personName,
          labelKey: "certificates.fields.staff",
          enableSorting: true,
          gridPrimary: true,
        }),
        createColumn<Certificate>({
          id: "type_uuid",
          accessorFn: (r) => localizedName(r.type_name, locale),
          labelKey: "certificates.fields.type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: [],
          cell: ({ row }) =>
            localizedName(row.original.type_name, locale) ||
            row.original.type_uuid ||
            "—",
        }),
        createColumn<Certificate>({
          accessorKey: "issued_at",
          labelKey: "certificates.fields.issued_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.issued_at ? format.date(row.original.issued_at) : "—",
        }),
        createColumn<Certificate>({
          accessorKey: "expires_at",
          labelKey: "certificates.fields.expires_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "expires",
          cell: ({ row }) =>
            row.original.expires_at
              ? format.date(row.original.expires_at)
              : "—",
        }),
        createColumn<Certificate>({
          accessorKey: "status",
          labelKey: "certificates.fields.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: CERTIFICATE_STATUSES.map((value) => ({
            value,
            labelKey: `certificates.status.${value}`,
            label: value,
          })),
          cell: ({ row }) => (
            <CertificateStatusChip status={row.original.status} />
          ),
        }),
        createColumn<Certificate>({
          id: "verified_by",
          accessorFn: (r) => r.verified_by_user_name ?? "",
          labelKey: "certificates.fields.verified_by",
          enableSorting: false,
          cell: ({ row }) => row.original.verified_by_user_name ?? "—",
        }),
        createColumn<Certificate>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "pdf",
                  label: t("certificates.actions.view_pdf"),
                  icon: FileText,
                  onSelect: async () =>
                    window.open(
                      URL.createObjectURL(
                        await certificatesService.downloadFile(
                          row.original.uuid,
                        ),
                      ),
                      "_blank",
                    ),
                },
                {
                  id: "verify",
                  label: t("certificates.actions.verify"),
                  icon: CheckCircle2,
                  permission: permissions.certificates.verify,
                  disabled: row.original.status !== "pending",
                  onSelect: () => verify.mutate(row.original.uuid),
                },
                {
                  id: "reject",
                  label: t("certificates.actions.reject"),
                  icon: XCircle,
                  permission: permissions.certificates.verify,
                  disabled: row.original.status !== "pending",
                  onSelect: () => setRejecting(row.original),
                },
                {
                  id: "revoke",
                  label: t("certificates.actions.revoke"),
                  icon: RotateCcw,
                  permission: permissions.certificates.write,
                  disabled: row.original.status !== "valid",
                  onSelect: () => revoke.mutate(row.original.uuid),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<Certificate, unknown>[],
    [format, locale, revoke, t, verify],
  );
  const list = useServerListState({
    columns,
    initialSort: "expires_at",
    persistKey: queue ? VERIFY_KEY : CERT_KEY,
    initialColumnFilters: queue
      ? [{ id: "status", value: ["pending"] }]
      : undefined,
  });
  const query = useQuery({
    queryKey: certificateKeys.list({
      ...list.params,
      ...(queue ? { status: "pending" } : {}),
    }),
    queryFn: () =>
      certificatesService.listCertificates({
        ...list.params,
        ...(queue ? { status: "pending" } : {}),
      }),
  });
  const rows = queue
    ? (query.data?.items ?? []).filter((r) => r.status === "pending")
    : (query.data?.items ?? []);
  const reject = useMutation({
    mutationFn: ({ uuid, reason }: { uuid: string; reason: string }) =>
      certificatesService.reject(uuid, reason),
    onSuccess: async () => {
      await invalidateAll(queryClient);
      setRejecting(null);
    },
    onError: (e) => toastError(e, t("certificates.toast.failed")),
  });
  return (
    <EntityPage
      title={
        queue ? t("certificates.verification.title") : t("certificates.title")
      }
      description={
        queue
          ? t("certificates.verification.description")
          : t("certificates.description")
      }
      permission={permissions.certificates.read}
      actions={
        !queue ? (
          <Button onClick={() => setUploadOpen(true)}>
            <Plus className="size-4" />
            {t("certificates.upload.title")}
          </Button>
        ) : null
      }
    >
      <EntityTable
        columns={columns}
        data={rows}
        rowCount={queue ? rows.length : (query.data?.total ?? 0)}
        state={list.tableState}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        features={{ persistKey: queue ? VERIFY_KEY : CERT_KEY }}
      />
      <UploadDialog open={uploadOpen} onOpenChange={setUploadOpen} />
      <DecisionDialog
        title={t("certificates.actions.reject")}
        open={Boolean(rejecting)}
        requireReason
        onOpenChange={(o) => !o && setRejecting(null)}
        onSubmit={(reason) =>
          rejecting && reject.mutate({ uuid: rejecting.uuid, reason })
        }
      />
    </EntityPage>
  );
}

export function CertificateApprovalsPage() {
  const { t, locale, format } = useLocale();
  const queryClient = useQueryClient();
  const [decision, setDecision] = useState<{
    row: CertificateWarning;
    approve: boolean;
  } | null>(null);
  const columns = useMemo(
    () =>
      [
        createColumn<CertificateWarning>({
          accessorKey: "service_no",
          labelKey: "certificates.fields.service",
          enableSorting: true,
          gridPrimary: true,
        }),
        createColumn<CertificateWarning>({
          accessorKey: "organization_name",
          labelKey: "certificates.fields.dealer",
          enableSorting: false,
        }),
        createColumn<CertificateWarning>({
          id: "user_name",
          accessorFn: personName,
          labelKey: "certificates.fields.staff",
          enableSorting: true,
        }),
        createColumn<CertificateWarning>({
          id: "type_name",
          accessorFn: (r) => localizedName(r.type_name, locale),
          labelKey: "certificates.fields.missing_type",
          enableSorting: false,
          cell: ({ row }) => localizedName(row.original.type_name, locale),
        }),
        createColumn<CertificateWarning>({
          accessorKey: "decision",
          labelKey: "certificates.fields.decision",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: WARNING_DECISIONS.map((value) => ({
            value,
            labelKey: `certificates.warning_decisions.${value}`,
            label: value,
          })),
          cell: ({ row }) => (
            <WarningDecisionChip decision={row.original.decision} />
          ),
        }),
        createColumn<CertificateWarning>({
          accessorKey: "created_at",
          labelKey: "accounting.fields.created_at",
          enableSorting: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<CertificateWarning>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "approve",
                  label: t("certificates.actions.approve"),
                  icon: CheckCircle2,
                  permission: permissions.certificates.approveService,
                  disabled: !["none", "pending_approval"].includes(
                    row.original.decision,
                  ),
                  onSelect: () =>
                    setDecision({ row: row.original, approve: true }),
                },
                {
                  id: "reject",
                  label: t("certificates.actions.reject"),
                  icon: XCircle,
                  permission: permissions.certificates.approveService,
                  disabled: !["none", "pending_approval"].includes(
                    row.original.decision,
                  ),
                  onSelect: () =>
                    setDecision({ row: row.original, approve: false }),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<CertificateWarning, unknown>[],
    [format, locale, t],
  );
  const list = useServerListState({
    columns,
    initialSort: "-created_at",
    persistKey: APPROVAL_KEY,
    initialColumnFilters: [{ id: "decision", value: ["pending_approval"] }],
  });
  const query = useQuery({
    queryKey: certificateKeys.warnings({
      ...list.params,
      decision: "pending_approval",
    }),
    queryFn: () =>
      certificatesService.listWarnings({
        ...list.params,
        decision: "pending_approval",
      }),
  });
  const rows = (query.data?.items ?? []).filter(
    (r) => r.decision === "pending_approval",
  );
  const decide = useMutation({
    mutationFn: ({
      uuid,
      approve,
      note,
    }: {
      uuid: string;
      approve: boolean;
      note: string;
    }) =>
      approve
        ? certificatesService.approveWarning(uuid, note)
        : certificatesService.rejectWarning(uuid, note),
    onSuccess: async () => {
      await invalidateAll(queryClient);
      setDecision(null);
    },
    onError: (e) => toastError(e, t("certificates.toast.failed")),
  });
  return (
    <EntityPage
      title={t("certificates.approvals.title")}
      description={t("certificates.approvals.description")}
      permission={permissions.certificates.approveService}
    >
      <EntityTable
        columns={columns}
        data={rows}
        rowCount={rows.length}
        state={list.tableState}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        features={{ persistKey: APPROVAL_KEY }}
      />
      <DecisionDialog
        title={
          decision?.approve
            ? t("certificates.actions.approve")
            : t("certificates.actions.reject")
        }
        open={Boolean(decision)}
        requireReason={decision?.approve === false}
        onOpenChange={(o) => !o && setDecision(null)}
        onSubmit={(note) =>
          decision &&
          decide.mutate({
            uuid: decision.row.uuid,
            approve: decision.approve,
            note,
          })
        }
      />
    </EntityPage>
  );
}
