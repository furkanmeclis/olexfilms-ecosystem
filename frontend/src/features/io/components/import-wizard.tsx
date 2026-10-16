"use client";

import { useCallback, useMemo, useState, type ReactNode } from "react";
import { useParams } from "next/navigation";

import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { routes } from "@/config/routes";
import {
  ImportColumnsStep,
  missingRequiredFields,
} from "@/features/io/components/import-columns-step";
import { ImportConfirmStep } from "@/features/io/components/import-confirm-step";
import { ImportFileStep } from "@/features/io/components/import-file-step";
import { ImportPreviewPanel } from "@/features/io/components/import-preview-panel";
import {
  ImportResultStep,
  type ImportResultLink,
} from "@/features/io/components/import-result-step";
import {
  detectImportFormat,
  readImportFileHead,
  type ImportFileHead,
} from "@/features/io/lib/import-file";
import {
  importConfirmCopy,
  importPlan,
  type ImportConfirmBuilder,
} from "@/features/io/lib/import-summary";
import {
  applyAutoDefaults,
  apiMappingToUi,
  MAPPING_SKIP,
  suggestColumnMapping,
  uiMappingToApi,
} from "@/features/io/lib/suggest-mapping";
import { importsService } from "@/features/io/services/imports.service";
import {
  IMPORT_PATHS,
  IMPORT_SCHEMA,
  type ExportJobScope,
  type ImportFormat,
  type ImportJob,
  type ImportResource,
  type IoResource,
} from "@/features/io/types";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";
import { cn } from "@/lib/utils";

/** Values of the resource-specific options and their setter. */
export type ImportWizardOptions = {
  values: Record<string, string>;
  setValue: (key: string, value: string) => void;
};

export type ImportWizardProps = {
  resource: ImportResource;
  /** Upload / sample endpoints of a record-scoped import (TEC-477). */
  paths?: { upload: string; sample: string };
  /** Job defaults the user does not edit (e.g. the target fleet). */
  fixedDefaults?: Record<string, string>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called when the job settled (applied, failed or rolled back). */
  onComplete?: () => void;
  /** Import job list (secondary link on the result step). */
  jobsHref?: string;
  scope?: ExportJobScope;
  /**
   * Resource-specific settings on the file step (e.g. the existing barcode
   * policy of TEC-760). Their values go with the upload form and into the
   * job defaults.
   */
  extraOptions?: (options: ImportWizardOptions) => ReactNode;
  initialOptions?: Record<string, string>;
  /** Overrides the confirmation copy of the resource. */
  confirmCopy?: ImportConfirmBuilder;
  /** Link to what an applied job created (default: the job detail only). */
  resultLink?: (job: ImportJob) => ImportResultLink | null;
  title?: string;
};

type Step = "file" | "columns" | "preview" | "confirm" | "result";

const STEPS: Step[] = ["file", "columns", "preview", "confirm", "result"];

function StepIndicator({ step }: { step: Step }) {
  const { t } = useLocale();
  const activeIndex = STEPS.indexOf(step);
  return (
    <ol
      className="flex flex-wrap items-center gap-2 text-xs"
      data-testid="import-steps"
    >
      {STEPS.map((s, index) => (
        <li key={s} className="flex items-center gap-2">
          <span
            aria-current={index === activeIndex ? "step" : undefined}
            className={cn(
              "flex items-center gap-1.5 rounded-full border px-2.5 py-1",
              index === activeIndex
                ? "border-primary bg-primary/10 text-primary font-medium"
                : index < activeIndex
                  ? "text-foreground border-border"
                  : "text-muted-foreground border-dashed",
            )}
          >
            <span className="tabular-nums">{index + 1}</span>
            {t(`imports.wizard_step.${s}`)}
          </span>
        </li>
      ))}
    </ol>
  );
}

/**
 * Import wizard v2 (TEC-758): File → Columns → Preview → Confirm → Result
 * on a wide sheet. Shared by every import (users, roles, catalog, fleet,
 * consumption expectations, recommended prices, stock).
 */
export function ImportWizard({
  resource,
  paths: pathsOverride,
  fixedDefaults,
  open,
  onOpenChange,
  onComplete,
  jobsHref,
  scope = "platform",
  extraOptions,
  initialOptions,
  confirmCopy,
  resultLink,
  title,
}: ImportWizardProps) {
  const { t, tp, locale } = useLocale();
  const params = useParams<{ slug?: string }>();
  const slug = typeof params?.slug === "string" ? params.slug : undefined;
  const paths = pathsOverride ?? IMPORT_PATHS[resource as IoResource];
  const schema = useMemo(() => IMPORT_SCHEMA[resource] ?? [], [resource]);

  const [step, setStep] = useState<Step>("file");
  const [file, setFile] = useState<File | null>(null);
  const [job, setJob] = useState<ImportJob | null>(null);
  const [head, setHead] = useState<ImportFileHead>({
    headers: [],
    samples: [],
  });
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [autoFields, setAutoFields] = useState<Set<string>>(new Set());
  const [defaults, setDefaults] = useState<Record<string, string>>({});
  const [options, setOptions] = useState<Record<string, string>>(
    initialOptions ?? {},
  );
  const [acknowledged, setAcknowledged] = useState(false);
  const [pending, setPending] = useState(false);
  const [mappingError, setMappingError] = useState<string | null>(null);

  const format = file ? detectImportFormat(file.name) : null;

  const reset = useCallback(() => {
    setStep("file");
    setFile(null);
    setJob(null);
    setHead({ headers: [], samples: [] });
    setMapping({});
    setAutoFields(new Set());
    setDefaults({});
    setOptions(initialOptions ?? {});
    setAcknowledged(false);
    setPending(false);
    setMappingError(null);
  }, [initialOptions]);

  const handleOpenChange = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };

  const label = useCallback((key: string) => t(key), [t]);

  const downloadSample = async (sampleFormat: ImportFormat) => {
    if (!paths) return;
    try {
      const blob = await importsService.downloadSample(
        paths.sample,
        sampleFormat,
        locale,
      );
      triggerBrowserDownload(blob, `sample.${sampleFormat}`);
    } catch {
      appToast.error(t("imports.toast.sample_failed"));
    }
  };

  const jobDefaults = () => ({ ...options, ...defaults, ...fixedDefaults });

  const handleUpload = async () => {
    if (!file || !format || !paths) return;
    setPending(true);
    try {
      const [created, fileHead] = await Promise.all([
        importsService.upload(paths.upload, file, format, locale, options),
        readImportFileHead(file, format),
      ]);
      const serverMapping = apiMappingToUi(created.mapping);
      const headers = [...fileHead.headers];
      for (const header of Object.values(serverMapping)) {
        if (!headers.includes(header)) headers.push(header);
      }
      const merged = {
        ...suggestColumnMapping(fileHead.headers, schema, label),
        ...serverMapping,
      };
      setHead({ headers, samples: fileHead.samples });
      setMapping(merged);
      setAutoFields(new Set(Object.keys(merged)));
      setDefaults((prev) => applyAutoDefaults(resource, merged, prev));
      setJob(created);
      setMappingError(null);
      setStep("columns");
    } catch {
      appToast.error(t("imports.toast.upload_failed"));
    } finally {
      setPending(false);
    }
  };

  const autoMatch = () => {
    const suggested = suggestColumnMapping(head.headers, schema, label);
    const count = Object.keys(suggested).length;
    if (!count) {
      appToast.info(t("imports.mapping_none"));
      return;
    }
    setMapping(suggested);
    setAutoFields(new Set(Object.keys(suggested)));
    setDefaults((prev) => applyAutoDefaults(resource, suggested, prev));
    appToast.success(t("imports.toast.auto_mapped", { count }));
  };

  const validateMapping = () => {
    const used = new Set<string>();
    for (const header of Object.values(mapping)) {
      if (!header || header === MAPPING_SKIP) continue;
      if (used.has(header)) {
        setMappingError(t("imports.mapping_duplicate"));
        return false;
      }
      used.add(header);
    }
    const missing = missingRequiredFields(
      schema,
      mapping,
      defaults,
      fixedDefaults,
    );
    if (missing.length) {
      setMappingError(
        t("imports.defaults_required", { count: missing.length }),
      );
      return false;
    }
    setMappingError(null);
    return true;
  };

  const handlePreview = async () => {
    if (!job || !validateMapping()) return;
    setPending(true);
    setStep("preview");
    try {
      const updated = await importsService.updateMapping(
        job.uuid,
        { mapping: uiMappingToApi(mapping), defaults: jobDefaults() },
        scope,
      );
      const previewed = await importsService.preview(updated.uuid, scope);
      setJob(previewed);
      setAcknowledged(false);
    } catch {
      appToast.error(t("imports.toast.preview_failed"));
      setStep("columns");
    } finally {
      setPending(false);
    }
  };

  const handleConfirm = async () => {
    if (!job) return;
    setPending(true);
    try {
      const confirmed = await importsService.confirm(job.uuid, scope);
      setJob(confirmed);
      setStep("result");
      appToast.success(t("imports.toast.confirmed"));
    } catch {
      appToast.error(t("imports.toast.confirm_failed"));
    } finally {
      setPending(false);
    }
  };

  const summary = job?.preview_summary;
  const plan = importPlan(summary);
  const copy = importConfirmCopy(resource, { summary, t, tp }, confirmCopy);
  const confirmBlocked =
    plan.importable === 0 || (plan.skipped > 0 && !acknowledged);

  const detailHref = job
    ? scope === "tenant" && slug
      ? routes.tenant.imports.detail(slug, job.uuid)
      : routes.platform.imports.detail(job.uuid)
    : "";
  const listHref =
    jobsHref ??
    (scope === "tenant" && slug
      ? routes.tenant.imports.root(slug)
      : routes.platform.imports.root);

  if (!paths) return null;

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent
        className="flex w-full flex-col gap-0 p-0 sm:max-w-5xl"
        data-testid="import-wizard"
      >
        <SheetHeader className="space-y-3 border-b px-6 py-4 text-start">
          <SheetTitle>{title ?? t("imports.wizard_title")}</SheetTitle>
          <SheetDescription>{t("imports.wizard_description")}</SheetDescription>
          <StepIndicator step={step} />
        </SheetHeader>

        <div className="flex-1 overflow-y-auto px-6 py-5">
          {step === "file" ? (
            <ImportFileStep
              file={file}
              format={format}
              onFile={setFile}
              onDownloadSample={(f) => void downloadSample(f)}
              extraOptions={extraOptions?.({
                values: options,
                setValue: (key, value) =>
                  setOptions((prev) => ({ ...prev, [key]: value })),
              })}
            />
          ) : null}

          {step === "columns" ? (
            <ImportColumnsStep
              resource={resource}
              schema={schema}
              headers={head.headers}
              samples={head.samples}
              mapping={mapping}
              autoFields={autoFields}
              defaults={defaults}
              fixedDefaults={fixedDefaults}
              error={mappingError}
              onMappingChange={(fieldKey, header) => {
                setMapping((prev) => ({ ...prev, [fieldKey]: header }));
                setAutoFields((prev) => {
                  const next = new Set(prev);
                  next.delete(fieldKey);
                  return next;
                });
              }}
              onDefaultChange={(fieldKey, value) =>
                setDefaults((prev) => ({ ...prev, [fieldKey]: value }))
              }
              onAutoMatch={autoMatch}
            />
          ) : null}

          {step === "preview" ? (
            pending || !summary || !job ? (
              <Loading label={t("imports.preview_loading")} />
            ) : (
              <ImportPreviewPanel
                summary={summary}
                schema={schema}
                jobUuid={job.uuid}
                scope={scope}
              />
            )
          ) : null}

          {step === "confirm" && summary ? (
            <ImportConfirmStep
              summary={summary}
              copy={copy}
              importable={plan.importable}
              skipped={plan.skipped}
              acknowledged={acknowledged}
              onAcknowledgedChange={setAcknowledged}
            />
          ) : null}

          {step === "result" && job ? (
            <ImportResultStep
              initialJob={job}
              scope={scope}
              detailHref={detailHref}
              jobsHref={listHref}
              resultLink={resultLink?.(job)}
              onSettled={() => onComplete?.()}
            />
          ) : null}
        </div>

        <div className="bg-background flex flex-wrap items-center justify-between gap-2 border-t px-6 py-3">
          <div>
            {step === "columns" || step === "preview" || step === "confirm" ? (
              <Button
                type="button"
                variant="outline"
                disabled={pending}
                onClick={() =>
                  setStep(
                    step === "columns"
                      ? "file"
                      : step === "preview"
                        ? "columns"
                        : "preview",
                  )
                }
              >
                {t("imports.back")}
              </Button>
            ) : null}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {step !== "result" ? (
              <Button
                type="button"
                variant="ghost"
                onClick={() => handleOpenChange(false)}
              >
                {t("imports.cancel")}
              </Button>
            ) : null}
            {step === "file" ? (
              <Button
                type="button"
                disabled={!file || !format || pending}
                onClick={() => void handleUpload()}
                data-testid="import-next"
              >
                {pending ? t("common.loading") : t("imports.next")}
              </Button>
            ) : null}
            {step === "columns" ? (
              <Button
                type="button"
                disabled={pending}
                onClick={() => void handlePreview()}
                data-testid="import-next"
              >
                {t("imports.preview_action")}
              </Button>
            ) : null}
            {step === "preview" ? (
              <Button
                type="button"
                disabled={pending || !summary}
                onClick={() => setStep("confirm")}
                data-testid="import-next"
              >
                {t("imports.next")}
              </Button>
            ) : null}
            {step === "confirm" ? (
              <Button
                type="button"
                disabled={pending || confirmBlocked}
                onClick={() => void handleConfirm()}
                data-testid="import-confirm"
              >
                {copy.action}
              </Button>
            ) : null}
            {step === "result" ? (
              <Button type="button" onClick={() => handleOpenChange(false)}>
                {t("common.close")}
              </Button>
            ) : null}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}
