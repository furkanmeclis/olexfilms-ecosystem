"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRightLeft, Check, CircleCheck } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { routes } from "@/config/routes";
import {
  PortalApiError,
  portalApi,
  type PortalVehicleTransfer,
} from "@/features/portal/lib/portal-client";
import { portalTransferError } from "@/features/portal/lib/portal-vehicles";
import {
  isTransferCode,
  pendingTransfer,
  verifyBody,
} from "@/features/vehicles/lib/transfer";
import { useLocale } from "@/providers/locale-provider";

export const portalTransferKey = (vehicleUuid: string) =>
  ["portal", "vehicles", "transfers", vehicleUuid] as const;

type ErrorMessage = { key: string; params?: Record<string, number> } | null;

function toMessage(err: unknown): ErrorMessage {
  if (err instanceof PortalApiError) return portalTransferError(err);
  return { key: "portal.transfer.error.generic" };
}

function PhoneStep({
  vehicleUuid,
  onStarted,
  onError,
}: {
  vehicleUuid: string;
  onStarted: (t: PortalVehicleTransfer) => void;
  onError: (err: unknown) => void;
}) {
  const { t } = useLocale();
  const [phone, setPhone] = useState("");
  const start = useMutation({
    mutationFn: () => portalApi.startVehicleTransfer(vehicleUuid, phone.trim()),
    onSuccess: onStarted,
    onError,
  });
  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (phone.trim()) start.mutate();
      }}
      data-testid="portal-transfer-phone-step"
    >
      <div className="space-y-1.5">
        <Label htmlFor="portal-transfer-phone">
          {t("portal.transfer.phone")}
        </Label>
        <Input
          id="portal-transfer-phone"
          type="tel"
          dir="ltr"
          autoComplete="off"
          value={phone}
          onChange={(e) => setPhone(e.target.value)}
          placeholder="+905551234567"
        />
        <p className="text-muted-foreground text-xs">
          {t("portal.transfer.phone_hint")}
        </p>
      </div>
      <Button type="submit" disabled={!phone.trim() || start.isPending}>
        <ArrowRightLeft className="size-4" />
        {t("portal.transfer.start")}
      </Button>
    </form>
  );
}

function CodeInput({
  id,
  label,
  hint,
  verified,
  value,
  onChange,
}: {
  id: string;
  label: string;
  hint?: string;
  verified: boolean;
  value: string;
  onChange: (v: string) => void;
}) {
  const { t } = useLocale();
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {verified ? (
        <p
          className="flex items-center gap-1 text-sm text-emerald-600"
          data-testid={`${id}-verified`}
        >
          <Check className="size-4" />
          {t("portal.transfer.verified")}
        </p>
      ) : (
        <Input
          id={id}
          inputMode="numeric"
          autoComplete="one-time-code"
          maxLength={6}
          dir="ltr"
          value={value}
          onChange={(e) => onChange(e.target.value.replace(/\D/g, ""))}
        />
      )}
      {hint && !verified ? (
        <p className="text-muted-foreground text-xs">{hint}</p>
      ) : null}
    </div>
  );
}

function CodeStep({
  transfer,
  onChanged,
  onCompleted,
  onError,
}: {
  transfer: PortalVehicleTransfer;
  onChanged: () => void;
  onCompleted: (t: PortalVehicleTransfer) => void;
  onError: (err: unknown) => void;
}) {
  const { t, format } = useLocale();
  const [fromCode, setFromCode] = useState("");
  const [toCode, setToCode] = useState("");
  const [name, setName] = useState("");
  const [surname, setSurname] = useState("");
  const verify = useMutation({
    mutationFn: () =>
      portalApi.verifyVehicleTransfer(
        transfer.uuid,
        verifyBody(transfer, { fromCode, toCode, name, surname }),
      ),
    onSuccess: (res) => {
      setFromCode("");
      setToCode("");
      if (res.status === "completed") onCompleted(res);
      else onChanged();
    },
    // A wrong code changes the attempt counter (or cancels the transfer).
    onError: (err) => {
      onError(err);
      onChanged();
    },
  });
  const cancel = useMutation({
    mutationFn: () => portalApi.cancelVehicleTransfer(transfer.uuid),
    onSuccess: onChanged,
    onError: (err) => {
      onError(err);
      onChanged();
    },
  });

  const fromReady = transfer.from_verified || isTransferCode(fromCode);
  const toReady = transfer.to_verified || isTransferCode(toCode);
  const anyCode =
    (!transfer.from_verified && isTransferCode(fromCode)) ||
    (!transfer.to_verified && isTransferCode(toCode));
  const needsName = !transfer.new_owner_known && fromReady && toReady;
  const canSubmit = anyCode && (!needsName || name.trim() !== "");

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (canSubmit) verify.mutate();
      }}
      data-testid="portal-transfer-code-step"
    >
      <div className="text-muted-foreground space-y-1 text-sm">
        <p>
          {t("portal.transfer.pending_info", {
            phone: transfer.to_phone_masked,
          })}
        </p>
        <p>
          {t("portal.transfer.expires", {
            time: format.dateTime(transfer.expires_at),
          })}
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <CodeInput
          id="portal-transfer-from-code"
          label={t("portal.transfer.from_code")}
          verified={transfer.from_verified}
          value={fromCode}
          onChange={setFromCode}
        />
        <CodeInput
          id="portal-transfer-to-code"
          label={t("portal.transfer.to_code")}
          hint={t("portal.transfer.to_code_hint")}
          verified={transfer.to_verified}
          value={toCode}
          onChange={setToCode}
        />
      </div>
      {!transfer.new_owner_known ? (
        <div className="space-y-2">
          <p className="text-muted-foreground text-xs">
            {t("portal.transfer.new_owner_hint")}
          </p>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="portal-transfer-name">
                {t("portal.transfer.new_owner_name")}
              </Label>
              <Input
                id="portal-transfer-name"
                value={name}
                maxLength={100}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="portal-transfer-surname">
                {t("portal.transfer.new_owner_surname")}
              </Label>
              <Input
                id="portal-transfer-surname"
                value={surname}
                maxLength={100}
                onChange={(e) => setSurname(e.target.value)}
              />
            </div>
          </div>
        </div>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button
          type="submit"
          disabled={!canSubmit || verify.isPending}
          data-testid="portal-transfer-verify"
        >
          {t("portal.transfer.verify")}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={cancel.isPending}
          onClick={() => cancel.mutate()}
          data-testid="portal-transfer-cancel"
        >
          {t("portal.transfer.cancel")}
        </Button>
      </div>
    </form>
  );
}

/**
 * The transfer steps (TEC-243): the buyer's phone, then the two codes
 * (the owner's own code and the code the buyer received), then done. An
 * open transfer of the vehicle resumes at the code step.
 */
export function PortalVehicleTransferFlow({
  vehicleUuid,
  onCompleted,
}: {
  vehicleUuid: string;
  onCompleted?: (t: PortalVehicleTransfer) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const key = portalTransferKey(vehicleUuid);
  const transfers = useQuery({
    queryKey: key,
    queryFn: () => portalApi.listVehicleTransfers(vehicleUuid),
  });
  const [error, setError] = useState<ErrorMessage>(null);
  const [done, setDone] = useState<PortalVehicleTransfer | null>(null);
  const pending = pendingTransfer(transfers.data?.items);
  const refresh = () => void qc.invalidateQueries({ queryKey: key });
  const fail = (err: unknown) => setError(toMessage(err));

  let body;
  if (done) {
    body = (
      <div
        className="flex flex-col items-center gap-2 py-4 text-center"
        data-testid="portal-transfer-done"
      >
        <CircleCheck className="size-10 text-emerald-600" />
        <p className="font-medium">{t("portal.transfer.done_title")}</p>
        <p className="text-muted-foreground text-sm">
          {t("portal.transfer.done", { count: done.warranties_moved })}
        </p>
      </div>
    );
  } else if (transfers.isLoading) {
    body = (
      <p className="text-muted-foreground text-sm">
        {t("portal.vehicles.loading")}
      </p>
    );
  } else if (pending) {
    body = (
      <CodeStep
        key={pending.uuid}
        transfer={pending}
        onChanged={refresh}
        onError={fail}
        onCompleted={(res) => {
          setError(null);
          setDone(res);
          onCompleted?.(res);
        }}
      />
    );
  } else {
    body = (
      <PhoneStep
        vehicleUuid={vehicleUuid}
        onError={fail}
        onStarted={(started) => {
          setError(null);
          qc.setQueryData(
            key,
            (old: { items: PortalVehicleTransfer[] } | undefined) => ({
              items: [started, ...(old?.items ?? [])],
            }),
          );
        }}
      />
    );
  }

  return (
    <div className="space-y-4" data-testid="portal-transfer-flow">
      {error ? (
        <p
          role="alert"
          className="border-destructive/40 bg-destructive/10 text-destructive rounded-md border px-3 py-2 text-sm"
          data-testid="portal-transfer-error"
        >
          {t(error.key, error.params)}
        </p>
      ) : null}
      {body}
    </div>
  );
}

/**
 * "Transfer vehicle" button and dialog of the portal vehicle detail
 * (TEC-243). After a completed transfer the vehicle is no longer the
 * user's: closing the dialog goes back to the vehicle list.
 */
export function PortalVehicleTransferDialog({
  vehicleUuid,
}: {
  vehicleUuid: string;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [completed, setCompleted] = useState(false);
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next && completed) {
          void qc.invalidateQueries({ queryKey: ["portal", "vehicles"] });
          router.replace(routes.portal.vehicles);
        }
      }}
    >
      <DialogTrigger asChild>
        <Button variant="outline" data-testid="portal-transfer-open">
          <ArrowRightLeft className="size-4" />
          {t("portal.transfer.button")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("portal.transfer.title")}</DialogTitle>
          <DialogDescription>
            {t("portal.transfer.description")}
          </DialogDescription>
        </DialogHeader>
        {open ? (
          <PortalVehicleTransferFlow
            vehicleUuid={vehicleUuid}
            onCompleted={() => setCompleted(true)}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
