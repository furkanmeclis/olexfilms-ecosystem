"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";

import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard } from "@/components/entity";
import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  einvoiceKeys,
  einvoiceService,
  type InvoiceProfile,
  type InvoiceProfileRequest,
} from "@/features/einvoice/services/einvoice.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const P = "einvoice.profile";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

export function invoiceProfileSchema(t: Translate) {
  return z
    .object({
      invoice_vkn: z
        .string()
        .trim()
        .regex(/^([0-9]{10})?$/, t(`${P}.validation.vkn`)),
      invoice_tckn: z
        .string()
        .trim()
        .regex(/^([0-9]{11})?$/, t(`${P}.validation.tckn`)),
      invoice_tax_office: z.string().trim().max(120),
      invoice_legal_name: z.string().trim().max(255),
      einvoice_registered: z.boolean(),
      einvoice_alias: z.string().trim().max(255),
      invoice_email: z.string().trim().max(255),
    })
    .superRefine((v, ctx) => {
      if (v.invoice_vkn && v.invoice_tckn) {
        ctx.addIssue({
          code: "custom",
          path: ["invoice_tckn"],
          message: t(`${P}.validation.one_id`),
        });
      }
    });
}

type ProfileValues = z.infer<ReturnType<typeof invoiceProfileSchema>>;

function defaults(p: InvoiceProfile): ProfileValues {
  return {
    invoice_vkn: p.invoice_vkn ?? "",
    invoice_tckn: p.invoice_tckn ?? "",
    invoice_tax_office: p.invoice_tax_office ?? "",
    invoice_legal_name: p.invoice_legal_name ?? "",
    einvoice_registered: p.einvoice_registered,
    einvoice_alias: p.einvoice_alias ?? "",
    invoice_email: p.invoice_email ?? "",
  };
}

function body(v: ProfileValues): InvoiceProfileRequest {
  const out: InvoiceProfileRequest = {
    einvoice_registered: v.einvoice_registered,
  };
  for (const key of [
    "invoice_vkn",
    "invoice_tckn",
    "invoice_tax_office",
    "invoice_legal_name",
    "einvoice_alias",
    "invoice_email",
  ] as const) {
    const value = v[key].trim();
    if (value) out[key] = value;
  }
  return out;
}

/** Codes meaning "not for this user / center": the card stays hidden. */
const HIDDEN_CODES = new Set([
  "FEATURE_DISABLED",
  "FORBIDDEN",
  "ORGANIZATION_CONTEXT_REQUIRED",
  "NO_TENANT_MEMBERSHIP",
  "NOT_FOUND",
]);

/**
 * Platform > organization detail "Fatura profili" (TEC-504): VKN / TCKN,
 * tax office, legal name, e-Fatura registration and alias of a distributor
 * or dealer, with what an invoice still misses. Shown to the brand
 * center's einvoice.read users while the e_invoice add-on is on (the
 * endpoint answers 403 otherwise and the card stays hidden); editing needs
 * einvoice.manage.
 */
export function InvoiceProfileCard({ orgUuid }: { orgUuid: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const allowed = can(permissions.einvoice.read);
  const editable = can(permissions.einvoice.manage);
  const query = useQuery({
    queryKey: einvoiceKeys.profile(orgUuid),
    queryFn: () => einvoiceService.getProfile(orgUuid),
    enabled: allowed,
    retry: false,
  });
  const save = useMutation({
    mutationFn: (v: ProfileValues) =>
      einvoiceService.putProfile(orgUuid, body(v)),
    onSuccess: (updated) => {
      qc.setQueryData(einvoiceKeys.profile(orgUuid), updated);
      appToast.success(t(`${P}.saved`));
    },
  });

  if (!allowed) return null;
  if (
    query.isError &&
    (!isApiError(query.error) ||
      query.error.status === 403 ||
      HIDDEN_CODES.has(query.error.code))
  ) {
    return null;
  }
  const profile = query.data;
  const missing = profile?.missing_fields ?? [];

  return (
    <EntitySectionCard
      title={t(`${P}.title`)}
      action={
        profile ? (
          <StatusChip
            label={
              missing.length === 0 ? t(`${P}.complete`) : t(`${P}.incomplete`)
            }
            tone={missing.length === 0 ? "success" : "warning"}
          />
        ) : undefined
      }
    >
      {!profile ? (
        <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
      ) : (
        <div className="space-y-4" data-testid="invoice-profile-card">
          {missing.length > 0 ? (
            <p className="text-sm text-amber-700 dark:text-amber-400">
              {t(`${P}.missing`, {
                fields: missing.map((f) => t(`${P}.fields.${f}`)).join(", "),
              })}
            </p>
          ) : null}
          <AppForm<ProfileValues>
            key={JSON.stringify(profile)}
            schema={invoiceProfileSchema(t)}
            defaultValues={defaults(profile)}
            onSubmit={async (v) => {
              await save.mutateAsync(v);
            }}
            className="space-y-4"
          >
            <div className="grid gap-4 md:grid-cols-2">
              <AppInput
                name="invoice_vkn"
                label={t(`${P}.fields.invoice_vkn`)}
                inputMode="numeric"
                dir="ltr"
                disabled={!editable}
              />
              <AppInput
                name="invoice_tckn"
                label={t(`${P}.fields.invoice_tckn`)}
                description={t(`${P}.id_hint`)}
                inputMode="numeric"
                dir="ltr"
                disabled={!editable}
              />
              <AppInput
                name="invoice_tax_office"
                label={t(`${P}.fields.invoice_tax_office`)}
                disabled={!editable}
              />
              <AppInput
                name="invoice_legal_name"
                label={t(`${P}.fields.invoice_legal_name`)}
                disabled={!editable}
              />
              <AppSwitch
                name="einvoice_registered"
                label={t(`${P}.fields.einvoice_registered`)}
                description={t(`${P}.registered_hint`)}
                disabled={!editable}
              />
              <AppInput
                name="einvoice_alias"
                label={t(`${P}.fields.einvoice_alias`)}
                placeholder="urn:mail:defaultpk@..."
                dir="ltr"
                disabled={!editable}
              />
              <AppInput
                name="invoice_email"
                label={t(`${P}.fields.invoice_email`)}
                type="email"
                dir="ltr"
                disabled={!editable}
              />
            </div>
            {editable ? (
              <div className="flex justify-end">
                <Button
                  type="submit"
                  size="sm"
                  disabled={save.isPending}
                  data-testid="invoice-profile-save"
                >
                  {t("common.save")}
                </Button>
              </div>
            ) : null}
          </AppForm>
        </div>
      )}
    </EntitySectionCard>
  );
}
