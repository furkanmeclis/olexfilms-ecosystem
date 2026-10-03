"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeftRight, ScanBarcode, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  addLine,
  buildCreateBody,
  lineIndexOfField,
  transferErrorMessage,
  validQuantity,
  type TransferLine,
} from "@/features/transfers/lib/transfers";
import {
  transferKeys,
  transfersService,
  type StockTransferKind,
} from "@/features/transfers/services/transfers.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Tenant > Transfers > new (TEC-197): pick a sibling (same parent, K13),
 * scan or type the barcodes of the units to hand over (a fixed barcode
 * takes a quantity), add a note and send the request. The server checks
 * every unit against the active organization's stock. With kind="return"
 * (TEC-223) the units go back to the direct parent, the only target.
 */
export function TransferFormPage({
  slug,
  kind = "sibling",
}: {
  slug: string;
  kind?: StockTransferKind;
}) {
  const isReturn = kind === "return";
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const qc = useQueryClient();
  const canCreate = can(Permission.TransfersRequest);
  const [target, setTarget] = useState("");
  const [scan, setScan] = useState("");
  const [lines, setLines] = useState<TransferLine[]>([]);
  const [note, setNote] = useState("");
  const [lineError, setLineError] = useState<{
    index: number;
    message: string;
  } | null>(null);

  const targets = useQuery({
    queryKey: isReturn ? transferKeys.targetsOf(kind) : transferKeys.targets,
    queryFn: () => transfersService.targets(isReturn ? kind : undefined),
    enabled: canCreate,
  });

  // A return has one possible target: the parent the server lists.
  const chosen = isReturn ? (targets.data?.items[0]?.uuid ?? "") : target;
  const body = buildCreateBody(chosen, lines, note, kind);
  const mutation = useMutation({
    mutationFn: () => {
      if (!body) throw new Error("incomplete");
      return transfersService.create(body);
    },
    onSuccess: (created) => {
      void qc.invalidateQueries({ queryKey: transferKeys.all });
      toast.success(
        t(isReturn ? "transfers.return.success" : "transfers.form.success"),
      );
      router.push(routes.tenant.transfers.detail(slug, created.uuid));
    },
    onError: (err) => {
      const message = transferErrorMessage(err, t, t("transfers.form.error"));
      const field = isApiError(err) ? err.details?.[0]?.field : undefined;
      const index = lineIndexOfField(field);
      setLineError(index >= 0 ? { index, message } : null);
      toast.error(message);
    },
  });

  const title = t(isReturn ? "transfers.return.title" : "transfers.form.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ArrowLeftRight className="size-6" />}
      description={t(
        isReturn
          ? "transfers.return.description"
          : "transfers.form.description",
      )}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("transfers.list.title"),
          href: routes.tenant.transfers.list(slug),
        },
        { label: title },
      ]}
    />
  );

  if (!canCreate) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("transfers.list.forbidden")}
        />
      </div>
    );
  }

  const siblings = targets.data?.items ?? [];

  const onScan = (e: FormEvent) => {
    e.preventDefault();
    setLines((ls) => addLine(ls, scan));
    setScan("");
    setLineError(null);
  };

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardHeader>
          <CardTitle>{t("transfers.form.target")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-1.5">
          {targets.isError ? (
            <p className="text-destructive text-sm">
              {t("common.error_generic")}
            </p>
          ) : !targets.isLoading && siblings.length === 0 ? (
            <p
              className="text-muted-foreground text-sm"
              data-testid="no-targets"
            >
              {t(
                isReturn
                  ? "transfers.return.no_parent"
                  : "transfers.form.no_targets",
              )}
            </p>
          ) : isReturn ? (
            <>
              <p className="font-medium" data-testid="return-parent">
                {siblings[0]?.name}
              </p>
              <p className="text-muted-foreground text-xs">
                {t("transfers.return.target_hint")}
              </p>
            </>
          ) : (
            <>
              <Label htmlFor="transfer-target">
                {t("transfers.form.target_label")}
              </Label>
              <Select value={target} onValueChange={setTarget}>
                <SelectTrigger
                  id="transfer-target"
                  data-testid="transfer-target"
                  className="w-full sm:w-96"
                >
                  <SelectValue
                    placeholder={t("transfers.form.target_placeholder")}
                  />
                </SelectTrigger>
                <SelectContent>
                  {siblings.map((o) => (
                    <SelectItem key={o.uuid} value={o.uuid}>
                      {o.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-muted-foreground text-xs">
                {t("transfers.form.target_hint")}
              </p>
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("transfers.form.units")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <form className="flex flex-wrap items-end gap-2" onSubmit={onScan}>
            <div className="min-w-60 flex-1 space-y-1.5">
              <Label htmlFor="transfer-scan">
                {t("transfers.form.barcode")}
              </Label>
              <Input
                id="transfer-scan"
                data-testid="transfer-scan"
                value={scan}
                dir="ltr"
                autoComplete="off"
                placeholder={t("transfers.form.barcode_placeholder")}
                onChange={(e) => setScan(e.target.value)}
              />
            </div>
            <Button
              type="submit"
              variant="outline"
              data-testid="transfer-add"
              disabled={scan.trim() === ""}
            >
              <ScanBarcode className="size-4" />
              {t("transfers.form.add")}
            </Button>
          </form>
          <p className="text-muted-foreground text-xs">
            {t("transfers.form.barcode_hint")}
          </p>
          {lines.length === 0 ? (
            <p
              className="text-muted-foreground text-sm"
              data-testid="transfer-lines-empty"
            >
              {t("transfers.form.no_units")}
            </p>
          ) : (
            <ul
              className="divide-y rounded-md border"
              data-testid="transfer-lines"
            >
              {lines.map((l, i) => (
                <li
                  key={l.barcode}
                  className="flex flex-wrap items-center gap-2 p-2"
                  data-testid="transfer-line"
                >
                  <span className="flex-1 font-mono text-sm" dir="ltr">
                    {l.barcode}
                  </span>
                  <Label className="sr-only" htmlFor={`qty-${i}`}>
                    {t("transfers.form.quantity")}
                  </Label>
                  <Input
                    id={`qty-${i}`}
                    className="w-32"
                    inputMode="numeric"
                    value={l.quantity}
                    aria-invalid={!validQuantity(l.quantity)}
                    placeholder={t("transfers.form.quantity_placeholder")}
                    onChange={(e) =>
                      setLines((ls) =>
                        ls.map((x, j) =>
                          j === i ? { ...x, quantity: e.target.value } : x,
                        ),
                      )
                    }
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    aria-label={t("transfers.form.remove")}
                    onClick={() => {
                      setLines((ls) => ls.filter((_, j) => j !== i));
                      setLineError(null);
                    }}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                  {lineError?.index === i ? (
                    <p
                      className="text-destructive w-full text-xs"
                      data-testid="transfer-line-error"
                    >
                      {lineError.message}
                    </p>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
          <div className="space-y-1.5">
            <Label htmlFor="transfer-note">{t("transfers.form.note")}</Label>
            <Textarea
              id="transfer-note"
              rows={3}
              maxLength={1000}
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => router.push(routes.tenant.transfers.list(slug))}
            >
              {t("transfers.form.cancel")}
            </Button>
            <Button
              type="button"
              data-testid="transfer-submit"
              disabled={!body || mutation.isPending}
              onClick={() => mutation.mutate()}
            >
              {t(
                isReturn ? "transfers.return.submit" : "transfers.form.submit",
              )}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
