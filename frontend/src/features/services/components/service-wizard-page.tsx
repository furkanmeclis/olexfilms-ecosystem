"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Wrench } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ContractStep } from "@/features/services/components/contract-step";
import { CustomerVehicleStep } from "@/features/services/components/customer-vehicle-step";
import { MeasurementStep } from "@/features/services/components/measurement-step";
import { IntakePhotosStep } from "@/features/photo-standard/components/intake-photos-step";
import {
  photoStandardKeys,
  photoStandardService,
} from "@/features/photo-standard/services/photo-standard.service";
import { PartsStep } from "@/features/services/components/parts-step";
import { StockStep } from "@/features/services/components/stock-step";
import {
  WizardActionsProvider,
  WizardBottomBar,
} from "@/features/services/components/wizard-bar";
import { serviceMeasurementKeys } from "@/features/measurements/services/service-measurements.service";
import { useFeature } from "@/features/modules/hooks/use-features";
import { resolveServiceWizardAccess } from "@/features/services/lib/access";
import { partsFromItems } from "@/features/services/lib/car-parts";
import {
  readStoredParts,
  storeParts,
} from "@/features/services/lib/parts-store";
import {
  canOpenStep,
  contractBlocksNext,
  nextStep,
  previousStep,
  wizardSteps,
  type WizardStep,
} from "@/features/services/lib/wizard";
import {
  serviceWizardKeys,
  serviceWizardService,
  type Service,
} from "@/features/services/services/service-wizard.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Service wizard (TEC-181, TEC-182): `uuid` undefined starts a new draft at
 * step 1; with a uuid the draft is loaded and step 1 shows it read-only.
 * Step 2 picks the parts (kept per draft in the browser and applied to the
 * items of step 4), step 4 adds the stock and completes the service.
 * TEC-291: the intake contract step sits before stock while the
 * intake_contracts module is on; a required contract keeps stock closed
 * until it is executed. TEC-500: with the photo_standard module the intake
 * photos follow step 1; a missing required angle keeps the later steps
 * closed, and a 422 PHOTO_STANDARD_INCOMPLETE from the contract or the
 * completion brings the photos back with the missing angles marked.
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
  const contracts = useFeature(slug, "intake_contracts");
  const certificates = useFeature(slug, "certificates");
  const photos = useFeature(slug, "photo_standard");
  const [step, setStep] = useState<WizardStep>(
    uuid ? "photos" : "customer_vehicle",
  );
  const [flagged, setFlagged] = useState<string[]>([]);
  // TEC-865: the bottom bar's action slot the steps portal their buttons to.
  const [actionsSlot, setActionsSlot] = useState<HTMLDivElement | null>(null);
  // null = not touched yet: the stored draft selection, else the parts the
  // items already carry.
  const [parts, setParts] = useState<string[] | null>(() =>
    uuid ? readStoredParts(uuid) : null,
  );

  const service = useQuery({
    queryKey: serviceWizardKeys.service(uuid ?? ""),
    queryFn: () => serviceWizardService.getService(uuid ?? ""),
    enabled: Boolean(uuid) && access.canStart,
  });
  const intake = useQuery({
    queryKey: photoStandardKeys.intake(uuid ?? ""),
    queryFn: () => photoStandardService.intake(uuid ?? ""),
    enabled: Boolean(uuid) && access.canStart && photos.enabled,
  });

  const title = t("services.wizard.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Wrench className="size-6" />}
      description={t("services.wizard.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("services.list.title"),
          href: can(permissions.services.read)
            ? routes.tenant.services.list(slug)
            : undefined,
        },
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
  // The contract step shows with the module, or whenever the service
  // itself already requires / carries a contract.
  const steps = wizardSteps(
    contracts.enabled ||
      Boolean(current?.contract_required) ||
      Boolean(current?.contract),
    photos.enabled,
  );
  const contractLocked = current ? contractBlocksNext(current) : false;
  const photosLocked =
    photos.enabled && (intake.data?.missing?.length ?? 0) > 0;
  // A hidden step falls to its neighbour; a locked step shows the step
  // that locks it (the photos first, then the contract).
  let shown: WizardStep = steps.includes(step)
    ? step
    : step === "photos"
      ? "parts"
      : "stock";
  if (
    hasService &&
    !canOpenStep(shown, hasService, false, steps, photosLocked)
  ) {
    shown = "photos";
  } else if (
    hasService &&
    !canOpenStep(shown, hasService, contractLocked, steps)
  ) {
    shown = "contract";
  }
  const photosIncomplete = (missing: string[]) => {
    setFlagged(missing);
    if (current) {
      void queryClient.invalidateQueries({
        queryKey: photoStandardKeys.intake(current.uuid),
      });
    }
    if (steps.includes("photos")) setStep("photos");
  };
  const goNext = (from: WizardStep) => {
    const n = nextStep(from, steps);
    if (n) setStep(n);
  };
  const goBack = (from: WizardStep) => {
    const p = previousStep(from, steps);
    if (p) setStep(p);
  };
  const stored = (saved: Service) => {
    queryClient.setQueryData(serviceWizardKeys.service(saved.uuid), saved);
  };
  const selectedParts = parts ?? partsFromItems(current?.items);
  const changeParts = (next: string[]) => {
    setParts(next);
    if (current) storeParts(current.uuid, next);
  };

  let body;
  if (shown === "customer_vehicle" || !current) {
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
  } else if (shown === "photos") {
    body = (
      <IntakePhotosStep
        serviceUuid={current.uuid}
        intake={intake}
        flagged={flagged}
        locked={
          current.status === "completed" || current.status === "cancelled"
        }
        onUploaded={(key) =>
          setFlagged((prev) => prev.filter((k) => k !== key))
        }
        onBack={() => goBack("photos")}
        onNext={() => goNext("photos")}
      />
    );
  } else if (shown === "measurement") {
    body = (
      <MeasurementStep
        slug={slug}
        key={current.updated_at}
        service={current}
        onBack={() => goBack("measurement")}
        onSaved={(saved) => {
          stored(saved);
          goNext("measurement");
        }}
      />
    );
  } else if (shown === "parts") {
    body = (
      <PartsStep
        service={current}
        selected={selectedParts}
        onChange={changeParts}
        onBack={() => goBack("parts")}
        onNext={() => goNext("parts")}
      />
    );
  } else if (shown === "contract") {
    body = (
      <ContractStep
        service={current}
        onBack={() => goBack("contract")}
        onNext={() => goNext("contract")}
        onPhotosIncomplete={photosIncomplete}
      />
    );
  } else if (shown === "stock") {
    body = (
      <StockStep
        service={current}
        selectedParts={selectedParts}
        onChanged={stored}
        onBack={() => goBack("stock")}
        onCompleted={(saved) => {
          stored(saved);
          storeParts(saved.uuid, null);
          // TEC-304: completion re-runs the before/after match; the links
          // the measurement step cached must not stay on the detail page.
          void queryClient.invalidateQueries({
            queryKey: serviceMeasurementKeys.links(saved.uuid),
          });
          void queryClient.invalidateQueries({
            queryKey: serviceMeasurementKeys.diff(saved.uuid),
          });
          router.push(routes.tenant.services.detail(slug, saved.uuid));
        }}
        showCertificateWarnings={certificates.enabled}
        onPhotosIncomplete={photosIncomplete}
      />
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <WizardActionsProvider value={actionsSlot}>
        <Card>
          <CardContent className="pt-6">{body}</CardContent>
        </Card>
      </WizardActionsProvider>
      <WizardBottomBar
        steps={steps}
        current={hasService ? shown : "customer_vehicle"}
        hasService={hasService}
        contractLocked={contractLocked}
        photosLocked={photosLocked}
        onSelect={setStep}
        actionsRef={setActionsSlot}
      />
    </div>
  );
}
