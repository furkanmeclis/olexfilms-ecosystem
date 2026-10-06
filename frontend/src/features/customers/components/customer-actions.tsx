"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { Download } from "lucide-react";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { customerErrorMessage } from "@/features/customers/lib/errors";
import {
  customerKeys,
  customersService,
  type CustomerAnonymizeResult,
  type CustomerDetail,
  type CustomerUpgrade,
  type DataExportFormat,
  type ExportJob,
} from "@/features/customers/services/customers.service";
import { useLocale } from "@/providers/locale-provider";

export type CustomerActionKind = "anonymize" | "export" | "upgrade";

/** The word typed to confirm the irreversible anonymization. */
export const ANONYMIZE_CONFIRM_WORD = "ANONYMIZE";

const selectClass =
  "border-input bg-background h-9 w-full rounded-md border px-2 text-sm";

/**
 * Anonymization (K19, irreversible): the user types the confirmation word;
 * the request goes through platformRequest, so the step-up dialog opens
 * when the backend asks for a fresh step-up.
 */
export function AnonymizeDialog({
  customer,
  open,
  onClose,
  onDone,
}: {
  customer: Pick<CustomerDetail, "uuid">;
  open: boolean;
  onClose: () => void;
  onDone: (result: CustomerAnonymizeResult) => void;
}) {
  const { t } = useLocale();
  const [word, setWord] = useState("");
  const run = useMutation({
    mutationFn: () => customersService.anonymize(customer.uuid),
    onSuccess: (result) => {
      toast.success(
        result.changed
          ? t("customers.actions.anonymize.success")
          : t("customers.actions.anonymize.already"),
      );
      setWord("");
      onDone(result);
    },
    onError: (err) =>
      toast.error(customerErrorMessage(err, t, t("customers.actions.failed"))),
  });
  const ok = word.trim().toUpperCase() === ANONYMIZE_CONFIRM_WORD;
  return (
    <Dialog open={open} onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("customers.actions.anonymize.title")}</DialogTitle>
          <DialogDescription>
            {t("customers.actions.anonymize.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="anonymize-confirm">
            {t("customers.actions.anonymize.type_word", {
              word: ANONYMIZE_CONFIRM_WORD,
            })}
          </Label>
          <Input
            id="anonymize-confirm"
            dir="ltr"
            autoComplete="off"
            value={word}
            onChange={(e) => setWord(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("customers.actions.back")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            data-testid="anonymize-confirm-button"
            disabled={!ok || run.isPending}
            onClick={() => run.mutate()}
          >
            {t("customers.actions.anonymize.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const POLL_MS = 2000;

/**
 * Personal data export (TEC-161): queue a JSON or PDF job, poll it and
 * download the file once completed (the download is audited server side).
 */
export function DataExportDialog({
  customer,
  open,
  onClose,
}: {
  customer: Pick<CustomerDetail, "uuid">;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const [format, setFormat] = useState<DataExportFormat>("pdf");
  const [jobUuid, setJobUuid] = useState<string | null>(null);
  const request = useMutation({
    mutationFn: () => customersService.requestDataExport(customer.uuid, format),
    onSuccess: (job) => {
      toast.success(t("customers.actions.export.queued"));
      setJobUuid(job.uuid);
    },
    onError: (err) =>
      toast.error(customerErrorMessage(err, t, t("customers.actions.failed"))),
  });
  const job = useQuery({
    queryKey: ["customers", "data-export", jobUuid ?? ""],
    queryFn: () => customersService.getDataExport(jobUuid ?? ""),
    enabled: open && jobUuid !== null,
    refetchInterval: (q) => {
      const status = (q.state.data as ExportJob | undefined)?.status;
      return status === "queued" || status === "processing" ? POLL_MS : false;
    },
  });
  const download = useMutation({
    mutationFn: (j: ExportJob) => customersService.downloadDataExport(j),
    onError: () => toast.error(t("customers.actions.export.download_failed")),
  });
  const status = job.data?.status ?? (jobUuid ? "queued" : null);

  return (
    <Dialog open={open} onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("customers.actions.export.title")}</DialogTitle>
          <DialogDescription>
            {t("customers.actions.export.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="export-format">
            {t("customers.actions.export.format")}
          </Label>
          <select
            id="export-format"
            className={selectClass}
            value={format}
            disabled={jobUuid !== null}
            onChange={(e) => setFormat(e.target.value as DataExportFormat)}
          >
            <option value="pdf">PDF</option>
            <option value="json">JSON</option>
          </select>
        </div>
        {status ? (
          <p
            className="text-sm"
            data-testid="export-status"
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
              data-testid="export-download"
              disabled={download.isPending}
              onClick={() => job.data && download.mutate(job.data)}
            >
              <Download className="size-4" />
              {t("customers.actions.export.download")}
            </Button>
          ) : (
            <Button
              type="button"
              data-testid="export-confirm"
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

/**
 * Upgrade to dealer: adds the customer as owner or staff of an existing
 * dealer in reach; the customer account, vehicles and history stay.
 */
export function UpgradeDialog({
  customer,
  open,
  onClose,
  onDone,
}: {
  customer: Pick<CustomerDetail, "uuid">;
  open: boolean;
  onClose: () => void;
  onDone: (result: CustomerUpgrade) => void;
}) {
  const { t } = useLocale();
  const [dealer, setDealer] = useState("");
  const [role, setRole] = useState<"dealer_owner" | "dealer_staff">(
    "dealer_staff",
  );
  const dealers = useQuery({
    queryKey: customerKeys.dealers,
    queryFn: () => customersService.listDealers(),
    enabled: open,
  });
  const run = useMutation({
    mutationFn: () =>
      customersService.upgrade(customer.uuid, {
        organization_uuid: dealer,
        role,
      }),
    onSuccess: (result) => {
      toast.success(
        t("customers.actions.upgrade.success", {
          dealer: result.organization.name,
        }),
      );
      onDone(result);
    },
    onError: (err) =>
      toast.error(customerErrorMessage(err, t, t("customers.actions.failed"))),
  });
  return (
    <Dialog open={open} onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("customers.actions.upgrade.title")}</DialogTitle>
          <DialogDescription>
            {t("customers.actions.upgrade.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="upgrade-dealer">
              {t("customers.actions.upgrade.dealer")}
            </Label>
            <select
              id="upgrade-dealer"
              className={selectClass}
              value={dealer}
              onChange={(e) => setDealer(e.target.value)}
            >
              <option value="">
                {dealers.isLoading
                  ? t("customers.loading")
                  : t("customers.actions.upgrade.dealer_placeholder")}
              </option>
              {(dealers.data ?? []).map((d) => (
                <option key={d.uuid} value={d.uuid}>
                  {d.name}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="upgrade-role">
              {t("customers.actions.upgrade.role")}
            </Label>
            <select
              id="upgrade-role"
              className={selectClass}
              value={role}
              onChange={(e) =>
                setRole(e.target.value as "dealer_owner" | "dealer_staff")
              }
            >
              <option value="dealer_staff">
                {t("customers.actions.upgrade.role_staff")}
              </option>
              <option value="dealer_owner">
                {t("customers.actions.upgrade.role_owner")}
              </option>
            </select>
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("customers.actions.back")}
          </Button>
          <Button
            type="button"
            data-testid="upgrade-confirm"
            disabled={!dealer || run.isPending}
            onClick={() => run.mutate()}
          >
            {t("customers.actions.upgrade.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
