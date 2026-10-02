"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { Download, FileDown } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { customerErrorMessage } from "@/features/customers/lib/errors";
import {
  customerKeys,
  customersService,
  type ExportJob,
  type ListExportFormat,
  type ListExportQuery,
} from "@/features/customers/services/customers.service";
import { useLocale } from "@/providers/locale-provider";

const POLL_MS = 2000;
const FORMATS: { value: ListExportFormat; label: string }[] = [
  { value: "csv", label: "CSV" },
  { value: "xlsx", label: "XLSX" },
  { value: "pdf", label: "PDF" },
];
const selectClass =
  "border-input bg-background h-9 w-full rounded-md border px-2 text-sm";

/**
 * Customer list export (TEC-199 on the TEC-164 endpoints): the button opens a
 * dialog, the user picks CSV / XLSX / PDF and the job is queued with the
 * list's current filters (q, status). The job is polled like the personal
 * data export and the file is downloaded (audited) once completed.
 */
export function CustomerListExportButton({
  filters,
}: {
  filters: ListExportQuery;
}) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        type="button"
        variant="outline"
        data-testid="customer-list-export"
        onClick={() => setOpen(true)}
      >
        <FileDown className="size-4" />
        {t("customers.list_export.button")}
      </Button>
      {open ? (
        <CustomerListExportDialog
          filters={filters}
          onClose={() => setOpen(false)}
        />
      ) : null}
    </>
  );
}

function CustomerListExportDialog({
  filters,
  onClose,
}: {
  filters: ListExportQuery;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const [format, setFormat] = useState<ListExportFormat>("xlsx");
  const [jobUuid, setJobUuid] = useState<string | null>(null);
  const request = useMutation({
    mutationFn: () => customersService.requestListExport(format, filters),
    onSuccess: (job) => {
      toast.success(t("customers.actions.export.queued"));
      setJobUuid(job.uuid);
    },
    onError: (err) =>
      toast.error(customerErrorMessage(err, t, t("customers.actions.failed"))),
  });
  const job = useQuery({
    queryKey: customerKeys.listExport(jobUuid ?? ""),
    queryFn: () => customersService.getListExport(jobUuid ?? ""),
    enabled: jobUuid !== null,
    refetchInterval: (q) => {
      const status = (q.state.data as ExportJob | undefined)?.status;
      return status === "queued" || status === "processing" ? POLL_MS : false;
    },
  });
  const download = useMutation({
    mutationFn: (j: ExportJob) => customersService.downloadListExport(j),
    onError: () => toast.error(t("customers.actions.export.download_failed")),
  });
  const status = job.data?.status ?? (jobUuid ? "queued" : null);

  return (
    <Dialog open onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("customers.list_export.title")}</DialogTitle>
          <DialogDescription>
            {t("customers.list_export.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="list-export-format">
            {t("customers.actions.export.format")}
          </Label>
          <select
            id="list-export-format"
            className={selectClass}
            value={format}
            disabled={jobUuid !== null}
            onChange={(e) => setFormat(e.target.value as ListExportFormat)}
          >
            {FORMATS.map((f) => (
              <option key={f.value} value={f.value}>
                {f.label}
              </option>
            ))}
          </select>
        </div>
        {status ? (
          <p
            className="text-sm"
            data-testid="list-export-status"
            data-status={status}
          >
            {t(`customers.actions.export.status.${status}`)}
            {status === "failed" && job.data?.error
              ? ` — ${job.data.error}`
              : ""}
          </p>
        ) : null}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("customers.actions.back")}
          </Button>
          {status === "completed" && job.data ? (
            <Button
              type="button"
              data-testid="list-export-download"
              disabled={download.isPending}
              onClick={() => job.data && download.mutate(job.data)}
            >
              <Download className="size-4" />
              {t("customers.actions.export.download")}
            </Button>
          ) : (
            <Button
              type="button"
              data-testid="list-export-confirm"
              disabled={
                request.isPending ||
                status === "queued" ||
                status === "processing"
              }
              onClick={() => request.mutate()}
            >
              {jobUuid
                ? t("customers.actions.export.retry")
                : t("customers.actions.export.confirm")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
