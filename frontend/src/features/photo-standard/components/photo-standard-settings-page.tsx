"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Camera, RotateCcw } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useFeature } from "@/features/modules/hooks/use-features";
import {
  angleName,
  overrideRows,
  overridesBody,
  resetOverride,
  setOverride,
  type OverrideRow,
} from "@/features/photo-standard/lib/photo-standard";
import {
  photoStandardKeys,
  photoStandardService,
  type PhotoAngle,
} from "@/features/photo-standard/services/photo-standard.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const T = "photo_standard.settings";

function defaultLabel(row: OverrideRow, t: (key: string) => string) {
  if (row.defaultHidden) return t(`${T}.default_hidden`);
  return row.defaultRequired
    ? t(`${T}.default_required`)
    : t(`${T}.default_optional`);
}

function OverrideForm({
  orgUuid,
  angles,
}: {
  orgUuid: string;
  angles: PhotoAngle[];
}) {
  const { t, locale } = useLocale();
  const queryClient = useQueryClient();
  const [rows, setRows] = useState<OverrideRow[]>(() => overrideRows(angles));
  const byKey = new Map(angles.map((a) => [a.key, a]));

  const save = useMutation({
    mutationFn: () =>
      photoStandardService.putOverrides(overridesBody(orgUuid, rows)),
    onSuccess: (res) => {
      queryClient.setQueryData(photoStandardKeys.overrides(orgUuid), res);
      setRows(overrideRows(res.items));
      appToast.success(t(`${T}.saved`));
    },
    onError: (error: unknown) =>
      appToast.error(isApiError(error) ? error.message : t(`${T}.save_failed`)),
  });

  const update = (key: string, fn: (row: OverrideRow) => OverrideRow) =>
    setRows((prev) => prev.map((r) => (r.key === key ? fn(r) : r)));

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ul className="divide-y rounded-lg border" data-testid="override-rows">
        {rows.map((row) => {
          const angle = byKey.get(row.key);
          const reqId = `override-${row.key}-required`;
          const hidId = `override-${row.key}-hidden`;
          return (
            <li
              key={row.key}
              className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center"
              data-testid="override-row"
              data-angle={row.key}
              data-overridden={row.overridden ? "true" : "false"}
            >
              <div className="min-w-0 flex-1 space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">
                    {angle ? angleName(angle, locale) : row.key}
                  </span>
                  {row.overridden ? (
                    <Badge variant="warning">{t(`${T}.overridden`)}</Badge>
                  ) : null}
                </div>
                <p
                  className="text-muted-foreground text-xs"
                  data-testid="override-default"
                >
                  {t(`${T}.center_default`, { value: defaultLabel(row, t) })}
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-4">
                <div className="flex items-center gap-2">
                  <Switch
                    id={reqId}
                    checked={row.required}
                    disabled={row.hidden}
                    onCheckedChange={(v) =>
                      update(row.key, (r) => setOverride(r, "required", v))
                    }
                    data-testid="override-required"
                  />
                  <Label htmlFor={reqId}>{t(`${T}.required`)}</Label>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    id={hidId}
                    checked={row.hidden}
                    onCheckedChange={(v) =>
                      update(row.key, (r) => setOverride(r, "hidden", v))
                    }
                    data-testid="override-hidden"
                  />
                  <Label htmlFor={hidId}>{t(`${T}.hidden`)}</Label>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={!row.overridden}
                  onClick={() => update(row.key, resetOverride)}
                  data-testid="override-reset"
                >
                  <RotateCcw className="size-4" />
                  {t(`${T}.reset`)}
                </Button>
              </div>
            </li>
          );
        })}
      </ul>
      <div className="flex justify-end">
        <Button
          type="submit"
          disabled={save.isPending}
          data-testid="override-save"
        >
          {t("common.save")}
        </Button>
      </div>
    </form>
  );
}

/**
 * Distributor "Fotoğraf standardı" settings (TEC-500): per central angle a
 * required and a hidden switch for the organization; untouched angles keep
 * following the center default shown next to them. Only overridden angles
 * are sent (PUT replaces the organization's overrides).
 */
export function PhotoStandardSettingsPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const feature = useFeature(slug, "photo_standard");
  const orgUuid = org?.uuid ?? "";
  const allowed = can(permissions.photoStandard.override);

  const overrides = useQuery({
    queryKey: photoStandardKeys.overrides(orgUuid),
    queryFn: () => photoStandardService.getOverrides(orgUuid),
    enabled: allowed && feature.enabled && orgUuid !== "",
  });

  const title = t(`${T}.title`);
  const header = (
    <PageHeader
      title={title}
      icon={<Camera className="size-6" />}
      description={t(`${T}.description`)}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
    />
  );

  let body;
  if (!allowed) {
    body = (
      <ErrorState
        title={t("common.error_forbidden")}
        description={t(`${T}.forbidden`)}
      />
    );
  } else if (feature.isLoading) {
    body = <Loading />;
  } else if (!feature.enabled) {
    body = (
      <ErrorState
        title={t("photo_standard.angles.module_off_title")}
        description={t(`${T}.module_off`)}
      />
    );
  } else if (overrides.isLoading) {
    body = <Loading />;
  } else if (overrides.isError || !overrides.data) {
    body = (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void overrides.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  } else if (overrides.data.items.length === 0) {
    body = (
      <p className="text-muted-foreground text-sm" data-testid="override-empty">
        {t(`${T}.empty`)}
      </p>
    );
  } else {
    body = (
      <OverrideForm
        key={overrides.dataUpdatedAt}
        orgUuid={orgUuid}
        angles={overrides.data.items}
      />
    );
  }

  return (
    <div className="space-y-6" data-testid="photo-standard-settings">
      {header}
      <Card>
        <CardContent className="pt-6">{body}</CardContent>
      </Card>
    </div>
  );
}
