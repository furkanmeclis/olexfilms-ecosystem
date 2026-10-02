"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRightLeft, Check } from "lucide-react";
import { useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  isTransferCode,
  pendingTransfer,
  transferStatusTone,
  verifyBody,
} from "@/features/vehicles/lib/transfer";
import {
  vehicleKeys,
  vehicleTransferService,
  type VehicleTransfer,
} from "@/features/vehicles/services/vehicle-transfer.service";
import { appToast } from "@/providers/toast-provider";
import { useLocale } from "@/providers/locale-provider";

const STATUS_LABELS: Record<string, string> = {
  pending: "vehicles.transfer.status.pending",
  completed: "vehicles.transfer.status.completed",
  cancelled: "vehicles.transfer.status.cancelled",
  expired: "vehicles.transfer.status.expired",
};

function StartForm({ vehicleUuid }: { vehicleUuid: string }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [phone, setPhone] = useState("");
  const start = useMutation({
    mutationFn: () => vehicleTransferService.start(vehicleUuid, phone.trim()),
    onSuccess: () => {
      appToast.success(t("vehicles.transfer.started"));
      setPhone("");
      void qc.invalidateQueries({
        queryKey: vehicleKeys.transfers(vehicleUuid),
      });
    },
  });
  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (phone.trim()) start.mutate();
      }}
      data-testid="transfer-start"
    >
      <div className="space-y-1.5">
        <Label htmlFor="transfer-phone">{t("vehicles.transfer.phone")}</Label>
        <Input
          id="transfer-phone"
          type="tel"
          dir="ltr"
          autoComplete="off"
          value={phone}
          onChange={(e) => setPhone(e.target.value)}
          placeholder="+905551234567"
        />
        <p className="text-muted-foreground text-xs">
          {t("vehicles.transfer.phone_hint")}
        </p>
      </div>
      <Button type="submit" disabled={!phone.trim() || start.isPending}>
        <ArrowRightLeft className="size-4" />
        {t("vehicles.transfer.start")}
      </Button>
    </form>
  );
}

function CodeField({
  id,
  label,
  verified,
  value,
  onChange,
}: {
  id: string;
  label: string;
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
          {t("vehicles.transfer.verified")}
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
    </div>
  );
}

function PendingForm({
  vehicleUuid,
  transfer,
}: {
  vehicleUuid: string;
  transfer: VehicleTransfer;
}) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const [fromCode, setFromCode] = useState("");
  const [toCode, setToCode] = useState("");
  const [name, setName] = useState("");
  const [surname, setSurname] = useState("");
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: vehicleKeys.transfers(vehicleUuid) });
    void qc.invalidateQueries({ queryKey: vehicleKeys.vehicle(vehicleUuid) });
  };
  const verify = useMutation({
    mutationFn: () =>
      vehicleTransferService.verify(
        transfer.uuid,
        verifyBody(transfer, { fromCode, toCode, name, surname }),
      ),
    onSuccess: (res) => {
      appToast.success(
        res.status === "completed"
          ? t("vehicles.transfer.completed", { count: res.warranties_moved })
          : t("vehicles.transfer.code_accepted"),
      );
      setFromCode("");
      setToCode("");
      refresh();
    },
    // Wrong codes change the attempt counter (or cancel the transfer).
    onError: refresh,
  });
  const cancel = useMutation({
    mutationFn: () => vehicleTransferService.cancel(transfer.uuid),
    onSuccess: () => {
      appToast.info(t("vehicles.transfer.cancelled"));
      refresh();
    },
    onError: refresh,
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
      data-testid="transfer-pending"
    >
      <div className="text-muted-foreground space-y-1 text-sm">
        <p>
          {t("vehicles.transfer.pending_info", {
            phone: transfer.to_phone_masked,
          })}
        </p>
        <p>
          {t("vehicles.transfer.expires", {
            time: format.dateTime(transfer.expires_at),
          })}
        </p>
        <p data-testid="transfer-attempts">
          {t("vehicles.transfer.attempts", {
            count: transfer.attempts,
            max: transfer.max_attempts,
          })}
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <CodeField
          id="transfer-from-code"
          label={t("vehicles.transfer.from_code")}
          verified={transfer.from_verified}
          value={fromCode}
          onChange={setFromCode}
        />
        <CodeField
          id="transfer-to-code"
          label={t("vehicles.transfer.to_code")}
          verified={transfer.to_verified}
          value={toCode}
          onChange={setToCode}
        />
      </div>
      {!transfer.new_owner_known ? (
        <div className="space-y-2">
          <p className="text-muted-foreground text-xs">
            {t("vehicles.transfer.new_owner_hint")}
          </p>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="transfer-name">
                {t("vehicles.transfer.new_owner_name")}
              </Label>
              <Input
                id="transfer-name"
                value={name}
                maxLength={100}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="transfer-surname">
                {t("vehicles.transfer.new_owner_surname")}
              </Label>
              <Input
                id="transfer-surname"
                value={surname}
                maxLength={100}
                onChange={(e) => setSurname(e.target.value)}
              />
            </div>
          </div>
        </div>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={!canSubmit || verify.isPending}>
          {t("vehicles.transfer.verify")}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={cancel.isPending}
          onClick={() => cancel.mutate()}
          data-testid="transfer-cancel"
        >
          {t("vehicles.transfer.cancel")}
        </Button>
      </div>
    </form>
  );
}

/**
 * Vehicle ownership transfer (TEC-190): the dealer enters the new owner's
 * phone, both owners receive a code over WhatsApp, the dealer types both
 * codes; the vehicle and its active warranties then move to the new owner.
 */
export function VehicleTransferCard({ vehicleUuid }: { vehicleUuid: string }) {
  const { t, format } = useLocale();
  const transfers = useQuery({
    queryKey: vehicleKeys.transfers(vehicleUuid),
    queryFn: () => vehicleTransferService.listTransfers(vehicleUuid),
    enabled: vehicleUuid !== "",
  });
  const items = transfers.data?.items ?? [];
  const pending = pendingTransfer(items);
  const past = items.filter((x) => x.status !== "pending");

  return (
    <Card data-testid="vehicle-transfer">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ArrowRightLeft className="text-muted-foreground size-4" />
          {t("vehicles.transfer.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <p className="text-muted-foreground text-sm">
          {t("vehicles.transfer.description")}
        </p>
        {transfers.isLoading ? null : pending ? (
          <PendingForm
            key={pending.uuid}
            vehicleUuid={vehicleUuid}
            transfer={pending}
          />
        ) : (
          <StartForm vehicleUuid={vehicleUuid} />
        )}
        {past.length > 0 ? (
          <div className="space-y-2">
            <h3 className="text-sm font-medium">
              {t("vehicles.transfer.history")}
            </h3>
            <ul className="space-y-1 text-sm" data-testid="transfer-history">
              {past.map((x) => (
                <li key={x.uuid} className="flex flex-wrap items-center gap-2">
                  <StatusChip
                    label={t(
                      STATUS_LABELS[x.status] ??
                        "vehicles.transfer.status.expired",
                    )}
                    tone={transferStatusTone(x.status)}
                  />
                  <span dir="ltr">{x.to_phone_masked}</span>
                  <span className="text-muted-foreground">
                    {format.dateTime(x.created_at)}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
