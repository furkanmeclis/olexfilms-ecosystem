"use client";

import { useCallback, useMemo, useState } from "react";
import Link from "next/link";
import { Sparkles } from "lucide-react";

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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { importsService } from "@/features/io/services/imports.service";
import {
  applyAutoDefaults,
  apiMappingToUi,
  countMappedFields,
  MAPPING_SKIP,
  suggestColumnMapping,
  uiMappingToApi,
} from "@/features/io/lib/suggest-mapping";
import {
  IMPORT_PATHS,
  IMPORT_SCHEMA,
  type ExportJobScope,
  type ImportFormat,
  type ImportJob,
  type IoResource,
} from "@/features/io/types";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";
import { cn } from "@/lib/utils";

type ImportWizardProps = {
  resource: IoResource;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onComplete?: () => void;
  jobsHref?: string;
  scope?: ExportJobScope;
};

type Step = "upload" | "mapping" | "defaults" | "preview" | "done";

const FORMATS: ImportFormat[] = ["xlsx", "csv", "tsv", "json"];
const STEPS: Step[] = ["upload", "mapping", "defaults", "preview", "done"];

function parseCsvHeaders(text: string, delimiter: string): string[] {
  const line = text.split(/\r?\n/).find((l) => l.trim());
  if (!line) return [];
  return line.split(delimiter).map((h) => h.trim().replace(/^"|"$/g, ""));
}

async function readFileHeaders(
  file: File,
  format: ImportFormat,
): Promise<string[]> {
  if (format === "csv") {
    const text = await file.slice(0, 4096).text();
    return parseCsvHeaders(text, ",");
  }
  if (format === "tsv") {
    const text = await file.slice(0, 4096).text();
    return parseCsvHeaders(text, "\t");
  }
  if (format === "json") {
    try {
      const text = await file.slice(0, 65536).text();
      const data = JSON.parse(text) as unknown;
      if (Array.isArray(data) && data[0] && typeof data[0] === "object") {
        return Object.keys(data[0] as Record<string, unknown>);
      }
    } catch {
      return [];
    }
  }
  return [];
}

function StepIndicator({ step, t }: { step: Step; t: (k: string) => string }) {
  const activeIndex = STEPS.indexOf(step);
  return (
    <ol className="flex flex-wrap gap-2 text-xs">
      {STEPS.map((s, index) => (
        <li
          key={s}
          className={cn(
            "rounded-full border px-2.5 py-0.5",
            index === activeIndex
              ? "border-primary bg-primary/10 text-primary font-medium"
              : index < activeIndex
                ? "text-muted-foreground border-border"
                : "text-muted-foreground/70 border-dashed",
          )}
        >
          {t(`imports.step.${s}`)}
        </li>
      ))}
    </ol>
  );
}

export function ImportWizard({
  resource,
  open,
  onOpenChange,
  onComplete,
  jobsHref,
  scope = "platform",
}: ImportWizardProps) {
  const { t, locale } = useLocale();
  const paths = IMPORT_PATHS[resource];
  const schema = useMemo(() => IMPORT_SCHEMA[resource] ?? [], [resource]);

  const [step, setStep] = useState<Step>("upload");
  const [format, setFormat] = useState<ImportFormat>("xlsx");
  const [file, setFile] = useState<File | null>(null);
  const [job, setJob] = useState<ImportJob | null>(null);
  const [headers, setHeaders] = useState<string[]>([]);
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [defaults, setDefaults] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const [mappingError, setMappingError] = useState<string | null>(null);

  const reset = useCallback(() => {
    setStep("upload");
    setFormat("xlsx");
    setFile(null);
    setJob(null);
    setHeaders([]);
    setMapping({});
    setDefaults({});
    setPending(false);
    setMappingError(null);
  }, []);

  const handleClose = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };

  const runAutoMatch = useCallback(
    (headerList: string[], baseMapping?: Record<string, string>) => {
      const suggested = suggestColumnMapping(headerList, schema, (key) =>
        t(key),
      );
      const merged = { ...baseMapping, ...suggested };
      const count = countMappedFields(merged);
      setMapping(merged);
      setDefaults((prev) => applyAutoDefaults(resource, merged, prev));
      if (count > 0) {
        appToast.success(t("imports.toast.auto_mapped", { count }));
      }
      return count;
    },
    [resource, schema, t],
  );

  const downloadSample = async (sampleFormat: ImportFormat) => {
    if (!paths) return;
    try {
      const blob = await importsService.downloadSample(
        paths.sample,
        sampleFormat,
        locale,
      );
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `sample.${sampleFormat === "json" ? "json" : sampleFormat}`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      appToast.error(t("imports.toast.sample_failed"));
    }
  };

  const handleUpload = async () => {
    if (!file || !paths) return;
    setPending(true);
    try {
      const created = await importsService.upload(
        paths.upload,
        file,
        format,
        locale,
      );
      const detected = await readFileHeaders(file, format);
      setHeaders(
        detected.length ? detected : Object.keys(created.mapping ?? {}),
      );

      let uiMapping = apiMappingToUi(created.mapping);
      if (detected.length) {
        const local = suggestColumnMapping(detected, schema, (key) => t(key));
        uiMapping = { ...local, ...uiMapping };
      }
      const matched = countMappedFields(uiMapping);
      setMapping(uiMapping);
      setDefaults(applyAutoDefaults(resource, uiMapping, {}));
      setJob(created);

      if (matched > 0) {
        appToast.success(t("imports.toast.auto_mapped", { count: matched }));
      } else if (format === "xlsx" && !detected.length) {
        appToast.info(t("imports.mapping_none"));
      }

      setStep("mapping");
    } catch {
      appToast.error(t("imports.toast.upload_failed"));
    } finally {
      setPending(false);
    }
  };

  const validateMapping = useCallback(() => {
    const used = new Map<string, string>();
    for (const [fieldKey, header] of Object.entries(mapping)) {
      if (!header || header === MAPPING_SKIP) continue;
      if (used.has(header)) {
        setMappingError(t("imports.mapping_duplicate"));
        return false;
      }
      used.set(header, fieldKey);
    }
    setMappingError(null);
    return true;
  }, [mapping, t]);

  const handleSaveMapping = async () => {
    if (!job || !validateMapping()) return;
    setPending(true);
    try {
      const updated = await importsService.updateMapping(
        job.uuid,
        {
          mapping: uiMappingToApi(mapping),
          defaults,
        },
        scope,
      );
      setJob(updated);
      setStep("defaults");
    } catch {
      appToast.error(t("imports.toast.mapping_failed"));
    } finally {
      setPending(false);
    }
  };

  const handleSaveDefaults = async () => {
    if (!job) return;
    setPending(true);
    try {
      const updated = await importsService.updateMapping(
        job.uuid,
        {
          mapping: uiMappingToApi(mapping),
          defaults,
        },
        scope,
      );
      setJob(updated);
      setStep("preview");
      const previewed = await importsService.preview(updated.uuid, scope);
      setJob(previewed);
    } catch {
      appToast.error(t("imports.toast.preview_failed"));
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
      setStep("done");
      appToast.success(t("imports.toast.confirmed"));
      onComplete?.();
    } catch {
      appToast.error(t("imports.toast.confirm_failed"));
    } finally {
      setPending(false);
    }
  };

  const preview = job?.preview_summary;
  const mappedCount = countMappedFields(mapping);

  const usedHeaders = useMemo(() => {
    const set = new Set<string>();
    for (const header of Object.values(mapping)) {
      if (header && header !== MAPPING_SKIP) set.add(header);
    }
    return set;
  }, [mapping]);

  const requiredDefaults = useMemo(
    () =>
      schema.filter(
        (field) =>
          field.required &&
          (!mapping[field.key] || mapping[field.key] === MAPPING_SKIP) &&
          !defaults[field.key],
      ),
    [schema, mapping, defaults],
  );

  if (!paths) return null;

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-h-[90vh] max-w-lg overflow-y-auto">
        <DialogHeader className="space-y-3">
          <DialogTitle>{t("imports.wizard_title")}</DialogTitle>
          <DialogDescription>
            {t("imports.wizard_description")}
          </DialogDescription>
          <StepIndicator step={step} t={t} />
        </DialogHeader>

        {step === "upload" ? (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("imports.format")}</Label>
              <Select
                value={format}
                onValueChange={(v) => setFormat(v as ImportFormat)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {FORMATS.map((f) => (
                    <SelectItem key={f} value={f}>
                      {t(`imports.formats.${f}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-muted-foreground text-xs">
                {t("imports.format_hint")}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="import-file">{t("imports.file")}</Label>
              <Input
                id="import-file"
                type="file"
                accept=".csv,.tsv,.xlsx,.json"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              />
              <p className="text-muted-foreground text-xs">
                {t("imports.file_hint")}
              </p>
            </div>
            <div className="space-y-2">
              <p className="text-sm font-medium">
                {t("imports.samples_title")}
              </p>
              <div className="flex flex-wrap gap-2">
                {FORMATS.map((f) => (
                  <Button
                    key={f}
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => void downloadSample(f)}
                  >
                    {t("imports.download_sample", {
                      format: t(`imports.formats.${f}`),
                    })}
                  </Button>
                ))}
              </div>
            </div>
          </div>
        ) : null}

        {step === "mapping" ? (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <p className="text-sm font-medium">
                  {t("imports.mapping_title")}
                </p>
                <p className="text-muted-foreground text-xs">
                  {t("imports.mapping_hint")}
                </p>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  const count = runAutoMatch(headers, {});
                  if (count === 0) appToast.info(t("imports.mapping_none"));
                }}
              >
                <Sparkles className="me-1.5 size-3.5" />
                {t("imports.mapping_auto")}
              </Button>
            </div>
            {mappedCount > 0 ? (
              <p className="text-muted-foreground text-xs">
                {t("imports.mapping_matched", { count: mappedCount })}
              </p>
            ) : null}
            {mappingError ? (
              <p className="text-destructive text-sm">{mappingError}</p>
            ) : null}
            {schema.map((field) => (
              <div key={field.key} className="grid gap-1.5">
                <Label>
                  {t(field.labelKey)}
                  {field.required ? " *" : ""}
                </Label>
                <Select
                  value={mapping[field.key] ?? MAPPING_SKIP}
                  onValueChange={(v) =>
                    setMapping((prev) => ({ ...prev, [field.key]: v }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t("imports.mapping_skip")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={MAPPING_SKIP}>
                      {t("imports.mapping_skip")}
                    </SelectItem>
                    {headers.map((header) => (
                      <SelectItem
                        key={header}
                        value={header}
                        disabled={
                          usedHeaders.has(header) &&
                          mapping[field.key] !== header
                        }
                      >
                        {header}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            ))}
            <p className="text-muted-foreground text-xs">
              {t("imports.mapping_auto_hint")}
            </p>
          </div>
        ) : null}

        {step === "defaults" ? (
          <div className="space-y-3">
            <div>
              <p className="text-sm font-medium">
                {t("imports.defaults_title")}
              </p>
              <p className="text-muted-foreground text-sm">
                {t("imports.defaults_hint")}
              </p>
            </div>
            {resource === "platform.users" ? (
              <p className="text-muted-foreground text-xs">
                {t("imports.defaults_users_status")} ·{" "}
                {t("imports.defaults_users_locale")}
              </p>
            ) : null}
            {schema.map((field) => (
              <div key={field.key} className="grid gap-1.5">
                <Label>{t(field.labelKey)}</Label>
                <Input
                  value={defaults[field.key] ?? ""}
                  placeholder={
                    mapping[field.key] && mapping[field.key] !== MAPPING_SKIP
                      ? t("imports.defaults_mapped")
                      : t("imports.defaults_optional")
                  }
                  disabled={Boolean(
                    mapping[field.key] && mapping[field.key] !== MAPPING_SKIP,
                  )}
                  onChange={(e) =>
                    setDefaults((prev) => ({
                      ...prev,
                      [field.key]: e.target.value,
                    }))
                  }
                />
              </div>
            ))}
            {requiredDefaults.length ? (
              <p className="text-destructive text-sm">
                {t("imports.defaults_required", {
                  count: requiredDefaults.length,
                })}
              </p>
            ) : null}
          </div>
        ) : null}

        {step === "preview" && preview ? (
          <div className="space-y-3 text-sm">
            <div>
              <p className="font-medium">{t("imports.preview_title")}</p>
              <p className="text-muted-foreground text-xs">
                {t("imports.preview_hint")}
              </p>
            </div>
            <div className="grid grid-cols-3 gap-2">
              <div className="rounded-md border p-2 text-center">
                <div className="font-medium">{preview.total}</div>
                <div className="text-muted-foreground">
                  {t("imports.preview_total")}
                </div>
              </div>
              <div className="rounded-md border p-2 text-center">
                <div className="font-medium text-green-600">
                  {preview.valid}
                </div>
                <div className="text-muted-foreground">
                  {t("imports.preview_valid")}
                </div>
              </div>
              <div className="rounded-md border p-2 text-center">
                <div className="text-destructive font-medium">
                  {preview.invalid}
                </div>
                <div className="text-muted-foreground">
                  {t("imports.preview_invalid")}
                </div>
              </div>
            </div>
            {(preview.errors?.length ?? 0) > 0 ? (
              <>
                <ul className="text-destructive max-h-32 list-disc overflow-y-auto ps-4">
                  {preview.errors?.slice(0, 10).map((err) => (
                    <li key={`${err.index}-${err.error}`}>
                      {t("imports.preview_row_error", {
                        row: err.index,
                        error: err.error,
                      })}
                    </li>
                  ))}
                </ul>
                {(preview.errors?.length ?? 0) > 10 ? (
                  <p className="text-muted-foreground text-xs">
                    {t("imports.preview_more_errors", {
                      count: (preview.errors?.length ?? 0) - 10,
                    })}
                  </p>
                ) : null}
              </>
            ) : null}
            <p className="text-muted-foreground text-xs">
              {t("imports.confirm_hint")}
            </p>
          </div>
        ) : null}

        {step === "done" ? (
          <div className="space-y-3 text-sm">
            <p className="font-medium">{t("imports.done_title")}</p>
            <p>{t("imports.done_message")}</p>
            <Button asChild variant="outline" size="sm">
              <Link href={jobsHref ?? routes.platform.imports.root}>
                {t("imports.view_jobs")}
              </Link>
            </Button>
          </div>
        ) : null}

        <DialogFooter className="gap-2 sm:gap-0">
          {step === "upload" ? (
            <>
              <Button
                type="button"
                variant="ghost"
                onClick={() => handleClose(false)}
              >
                {t("imports.cancel")}
              </Button>
              <Button
                type="button"
                disabled={!file || pending}
                onClick={() => void handleUpload()}
              >
                {pending ? t("common.loading") : t("imports.next")}
              </Button>
            </>
          ) : null}
          {step === "mapping" ? (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => setStep("upload")}
              >
                {t("imports.back")}
              </Button>
              <Button
                type="button"
                disabled={pending}
                onClick={() => void handleSaveMapping()}
              >
                {t("imports.next")}
              </Button>
            </>
          ) : null}
          {step === "defaults" ? (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => setStep("mapping")}
              >
                {t("imports.back")}
              </Button>
              <Button
                type="button"
                disabled={pending || requiredDefaults.length > 0}
                onClick={() => void handleSaveDefaults()}
              >
                {t("imports.preview_action")}
              </Button>
            </>
          ) : null}
          {step === "preview" ? (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => setStep("defaults")}
              >
                {t("imports.back")}
              </Button>
              <Button
                type="button"
                disabled={pending}
                onClick={() => void handleConfirm()}
              >
                {t("imports.confirm")}
              </Button>
            </>
          ) : null}
          {step === "done" ? (
            <Button type="button" onClick={() => handleClose(false)}>
              {t("common.close")}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
