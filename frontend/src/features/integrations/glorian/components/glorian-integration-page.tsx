"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PlugZap, Warehouse } from "lucide-react";
import { useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { GlorianConnectionForm } from "@/features/integrations/glorian/components/glorian-connection-form";
import { GlorianOutbounds } from "@/features/integrations/glorian/components/glorian-outbounds";
import { GlorianReconcile } from "@/features/integrations/glorian/components/glorian-reconcile";
import { GlorianSyncRuns } from "@/features/integrations/glorian/components/glorian-sync-runs";
import { glorianErrorText } from "@/features/integrations/glorian/lib/errors";
import {
  toPutBody,
  type GlorianFormValues,
} from "@/features/integrations/glorian/lib/form";
import { glorianKeys } from "@/features/integrations/glorian/lib/keys";
import {
  glorianService,
  type GlorianConnection,
  type GlorianTestResult,
} from "@/features/integrations/glorian/services/glorian.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Platform > Integrations > Glorian (TEC-274): the hub connection of the
 * glorian brand, its sync runs, held order outbounds and the reconcile
 * drift report. `integrations.glorian.view` reads, `.manage` writes.
 */
export function GlorianIntegrationPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canView = can(permissions.integrations.glorian.view);
  const canManage = can(permissions.integrations.glorian.manage);

  const header = (
    <PageHeader
      icon={<Warehouse className="size-7" />}
      title={t("integrations.glorian.title")}
      description={t("integrations.glorian.description")}
    />
  );

  if (!canView) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState title={t("integrations.glorian.forbidden")} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <GlorianSections canManage={canManage} />
    </div>
  );
}

function GlorianSections({ canManage }: { canManage: boolean }) {
  const { t } = useLocale();
  const queryClient = useQueryClient();

  const conn = useQuery({
    queryKey: glorianKeys.connection(),
    queryFn: () => glorianService.get(),
  });

  const save = useMutation({
    mutationFn: (values: GlorianFormValues) =>
      glorianService.save(toPutBody(values)),
    onSuccess: async (data) => {
      queryClient.setQueryData(glorianKeys.connection(), data);
      await queryClient.invalidateQueries({ queryKey: glorianKeys.all });
      appToast.success(t("integrations.glorian.toast.saved"));
    },
    onError: (error) => appToast.error(glorianErrorText(t, error)),
  });

  if (conn.isLoading) return <Loading label={t("common.loading")} />;
  if (conn.isError || !conn.data) {
    return (
      <ErrorState
        title={t("integrations.glorian.error.title")}
        description={glorianErrorText(t, conn.error)}
        retryLabel={t("common.retry")}
        onRetry={() => conn.refetch()}
      />
    );
  }

  const data = conn.data;

  return (
    <>
      <Section
        title={t("integrations.glorian.connection.title")}
        description={t("integrations.glorian.connection.description")}
        badges={<ConnectionBadges connection={data} />}
      >
        <div className="space-y-6">
          <GlorianConnectionForm
            connection={data}
            canManage={canManage}
            isSaving={save.isPending}
            onSubmit={async (values) => {
              await save.mutateAsync(values);
            }}
          />
          {canManage && data.configured ? <TestConnection /> : null}
        </div>
      </Section>

      {data.configured ? (
        <>
          <Section
            title={t("integrations.glorian.runs.title")}
            description={t("integrations.glorian.runs.description")}
          >
            <GlorianSyncRuns canManage={canManage} />
          </Section>
          <Section
            title={t("integrations.glorian.outbounds.title")}
            description={t("integrations.glorian.outbounds.description")}
          >
            <GlorianOutbounds canManage={canManage} />
          </Section>
          <Section
            title={t("integrations.glorian.reconcile.title")}
            description={t("integrations.glorian.reconcile.description")}
          >
            <GlorianReconcile canManage={canManage} />
          </Section>
        </>
      ) : (
        <Alert data-testid="glorian-not-configured">
          <AlertDescription>
            {t("integrations.glorian.not_configured")}
          </AlertDescription>
        </Alert>
      )}
    </>
  );
}

function Section({
  title,
  description,
  badges,
  children,
}: {
  title: string;
  description: string;
  badges?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>{title}</CardTitle>
          {badges}
        </div>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

function ConnectionBadges({ connection }: { connection: GlorianConnection }) {
  const { t } = useLocale();
  if (!connection.configured) {
    return (
      <Badge variant="outline">
        {t("integrations.glorian.connection.not_configured")}
      </Badge>
    );
  }
  return (
    <div className="flex flex-wrap gap-2">
      <Badge variant={connection.active ? "success" : "secondary"}>
        {connection.active
          ? t("integrations.glorian.connection.active")
          : t("integrations.glorian.connection.inactive")}
      </Badge>
      <Badge variant={connection.api_key_set ? "outline" : "warning"}>
        {connection.api_key_set
          ? t("integrations.glorian.connection.key_set")
          : t("integrations.glorian.connection.key_missing")}
      </Badge>
    </div>
  );
}

function TestConnection() {
  const { t } = useLocale();
  const [result, setResult] = useState<GlorianTestResult | null>(null);
  const test = useMutation({
    mutationFn: () => glorianService.test(),
    onSuccess: (res) => setResult(res),
    onError: (error) => {
      setResult(null);
      appToast.error(glorianErrorText(t, error));
    },
  });

  return (
    <div className="flex flex-wrap items-center gap-3 border-t pt-4">
      <Button
        type="button"
        variant="outline"
        onClick={() => test.mutate()}
        disabled={test.isPending}
        data-testid="glorian-test"
      >
        <PlugZap className="size-4" aria-hidden />
        {test.isPending
          ? t("integrations.glorian.test.running")
          : t("integrations.glorian.test.button")}
      </Button>
      {result ? (
        <div className="text-sm" data-testid="glorian-test-result">
          {result.ok ? (
            <Badge variant="success">
              {t("integrations.glorian.test.ok", { ms: result.duration_ms })}
            </Badge>
          ) : (
            <span className="flex flex-wrap items-center gap-2">
              <Badge variant="danger">
                {t("integrations.glorian.test.failed")}
              </Badge>
              <span className="font-mono text-xs">
                {[
                  result.code,
                  result.http_status ? `HTTP ${result.http_status}` : null,
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </span>
              {result.message ? (
                <span className="text-muted-foreground break-all">
                  {result.message}
                </span>
              ) : null}
            </span>
          )}
        </div>
      ) : null}
    </div>
  );
}
