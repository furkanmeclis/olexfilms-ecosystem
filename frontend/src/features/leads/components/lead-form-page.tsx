"use client";

import { useMutation } from "@tanstack/react-query";
import { Flame } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { LeadFields } from "@/features/leads/components/lead-fields";
import {
  createBody,
  emptyLeadForm,
  validateLeadForm,
  type LeadFormValues,
} from "@/features/leads/lib/leads";
import { leadsService } from "@/features/leads/services/leads.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export function LeadFormPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const canWrite = can(permissions.leads.write);
  const [values, setValues] = useState<LeadFormValues>(() => emptyLeadForm());
  const [errors, setErrors] = useState<Record<string, string>>({});
  const create = useMutation({
    mutationFn: () => leadsService.create(createBody(values)),
    onSuccess: (lead) => {
      toast.success(t("leads.form.created"));
      router.push(routes.tenant.leads.detail(slug, lead.uuid));
    },
    onError: () => toast.error(t("leads.form.error")),
  });

  const title = t("leads.form.new_title");
  const header = (
    <PageHeader
      title={title}
      icon={<Flame className="size-6" />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("leads.list.title"), href: routes.tenant.leads.list(slug) },
        { label: title },
      ]}
    />
  );

  if (!canWrite) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("leads.list.forbidden")}
        />
      </div>
    );
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    const local = validateLeadForm(values);
    if (Object.keys(local).length > 0) {
      setErrors(
        Object.fromEntries(Object.entries(local).map(([k, v]) => [k, t(v)])),
      );
      return;
    }
    setErrors({});
    create.mutate();
  };

  return (
    <form className="space-y-6" onSubmit={onSubmit} noValidate>
      {header}
      <Card>
        <CardContent className="space-y-6 pt-6">
          <LeadFields
            values={values}
            errors={errors}
            disabled={create.isPending}
            onChange={(p) => setValues((v) => ({ ...v, ...p }))}
          />
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => router.push(routes.tenant.leads.list(slug))}
            >
              {t("leads.form.cancel")}
            </Button>
            <Button
              type="submit"
              data-testid="lead-submit"
              disabled={create.isPending}
            >
              {t("leads.form.create")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </form>
  );
}
