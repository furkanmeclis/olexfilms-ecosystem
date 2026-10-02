"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Wrench } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { CustomerVehicleStep } from "@/features/services/components/customer-vehicle-step";
import { MeasurementStep } from "@/features/services/components/measurement-step";
import { resolveServiceWizardAccess } from "@/features/services/lib/access";
import {
  PLACEHOLDER_STEPS,
  WIZARD_STEPS,
  canOpenStep,
  nextStep,
  previousStep,
  type WizardStep,
} from "@/features/services/lib/wizard";
import {
  serviceWizardKeys,
  serviceWizardService,
  type Service,
} from "@/features/services/services/service-wizard.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Stepper({
  current,
  hasService,
  onSelect,
}: {
  current: WizardStep;
  hasService: boolean;
  onSelect: (step: WizardStep) => void;
}) {
  const { t } = useLocale();
  const currentIndex = WIZARD_STEPS.indexOf(current);
  return (
    <ol className="grid gap-2 sm:grid-cols-4" data-testid="wizard-stepper">
      {WIZARD_STEPS.map((step, i) => {
        const active = step === current;
        const done = hasService && i < currentIndex;
        const open = canOpenStep(step, hasService);
        return (
          <li key={step}>
            <button
              type="button"
              disabled={!open}
              aria-current={active ? "step" : undefined}
              data-step={step}
              onClick={() => onSelect(step)}
              className={cn(
                "flex w-full items-center gap-3 rounded-lg border p-3 text-start text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50",
                active ? "border-primary bg-primary/5" : "hover:bg-accent",
              )}
            >
              <span
                className={cn(
                  "flex size-7 shrink-0 items-center justify-center rounded-full border text-xs font-semibold",
                  active && "border-primary bg-primary text-primary-foreground",
                  done && "border-primary text-primary",
                )}
              >
                {done ? <Check className="size-4" /> : i + 1}
              </span>
              <span className="font-medium">
                {t(`services.wizard.steps.${step}`)}
              </span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}

function PlaceholderStep({
  step,
  onBack,
  onNext,
}: {
  step: WizardStep;
  onBack: () => void;
  onNext: (() => void) | null;
}) {
  const { t } = useLocale();
  return (
    <div className="space-y-6" data-testid={`placeholder-${step}`}>
      <div className="border-border text-muted-foreground rounded-lg border border-dashed px-6 py-12 text-center text-sm">
        {t("services.wizard.placeholder")}
      </div>
      <div className="flex flex-wrap justify-between gap-2">
        <Button type="button" variant="outline" onClick={onBack}>
          {t("services.wizard.back")}
        </Button>
        {onNext ? (
          <Button type="button" onClick={onNext}>
            {t("services.wizard.next")}
          </Button>
        ) : null}
      </div>
    </div>
  );
}

/**
 * Service wizard (TEC-181): `uuid` undefined starts a new draft at step 1;
 * with a uuid the draft is loaded and step 1 shows it read-only.
 */
export function ServiceWizardPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid?: string;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { can } = usePermission();
  const access = resolveServiceWizardAccess(can);
  const [step, setStep] = useState<WizardStep>(
    uuid ? "parts" : "customer_vehicle",
  );

  const service = useQuery({
    queryKey: serviceWizardKeys.service(uuid ?? ""),
    queryFn: () => serviceWizardService.getService(uuid ?? ""),
    enabled: Boolean(uuid) && access.canStart,
  });

  const title = t("services.wizard.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Wrench className="size-6" />}
      description={t("services.wizard.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("services.nav") },
        { label: title },
      ]}
      actions={
        service.data ? (
          <Badge variant="secondary" data-testid="service-no">
            <span dir="ltr">{service.data.service_no}</span>
            <span className="ms-2">{service.data.status_label}</span>
          </Badge>
        ) : null
      }
    />
  );

  if (!access.canStart) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("services.wizard.forbidden")}
        />
      </div>
    );
  }
  if (uuid && service.isLoading) return <Loading />;
  if (uuid && (service.isError || !service.data)) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void service.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }

  const current = service.data ?? null;
  const hasService = Boolean(current);
  const goNext = (from: WizardStep) => {
    const n = nextStep(from);
    if (n) setStep(n);
  };
  const goBack = (from: WizardStep) => {
    const p = previousStep(from);
    if (p) setStep(p);
  };
  const stored = (saved: Service) => {
    queryClient.setQueryData(serviceWizardKeys.service(saved.uuid), saved);
  };

  let body;
  if (step === "customer_vehicle" || !current) {
    body = (
      <CustomerVehicleStep
        access={access}
        service={current}
        onDone={(saved) => {
          stored(saved);
          if (!current) {
            router.replace(routes.tenant.services.wizard(slug, saved.uuid));
            return;
          }
          goNext("customer_vehicle");
        }}
      />
    );
  } else if (step === "measurement") {
    body = (
      <MeasurementStep
        key={current.updated_at}
        service={current}
        onBack={() => goBack("measurement")}
        onSaved={(saved) => {
          stored(saved);
          goNext("measurement");
        }}
      />
    );
  } else if (PLACEHOLDER_STEPS.has(step)) {
    const n = nextStep(step);
    body = (
      <PlaceholderStep
        step={step}
        onBack={() => goBack(step)}
        onNext={n ? () => setStep(n) : null}
      />
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <Stepper
        current={hasService ? step : "customer_vehicle"}
        hasService={hasService}
        onSelect={setStep}
      />
      <Card>
        <CardContent className="pt-6">{body}</CardContent>
      </Card>
    </div>
  );
}
