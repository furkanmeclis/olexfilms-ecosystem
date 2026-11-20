"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Download, FileKey2 } from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
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
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { glorianErrorText } from "@/features/integrations/glorian/lib/errors";
import { glorianKeys } from "@/features/integrations/glorian/lib/keys";
import {
  glorianService,
  type GlorianConnection,
  type GlorianConnectionPackage,
} from "@/features/integrations/glorian/services/glorian.service";
import { appToast } from "@/lib/toast";
import { useFormatter, useLocale } from "@/providers/locale-provider";

const MIN_PASSWORD = 12;
const IPV4 = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/;

/** Splits the IP field on commas, spaces and new lines. */
export function parseIpList(raw: string): string[] {
  return raw
    .split(/[\s,;]+/)
    .map((ip) => ip.trim())
    .filter(Boolean);
}

/** Loose client check; the backend validates with net.ParseIP. */
export function isIpAddress(ip: string): boolean {
  return IPV4.test(ip) || (ip.includes(":") && /^[0-9a-fA-F:.]+$/.test(ip));
}

function downloadPackage(pkg: GlorianConnectionPackage) {
  const blob = new Blob([JSON.stringify(pkg.package, null, 4) + "\n"], {
    type: "application/json",
  });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = pkg.filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/**
 * TEC-1265: "Bağlantı paketi oluştur". Issues a new API key sealed in a
 * password-encrypted file for the Glorian panel. The password is only sent
 * once and cleared when the dialog closes; the plain key never reaches
 * the browser.
 */
export function GlorianConnectionPackageButton({
  connection,
}: {
  connection: GlorianConnection;
}) {
  const { t } = useLocale();
  const format = useFormatter();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [baseUrl, setBaseUrl] = useState(connection.base_url);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [dealerWrite, setDealerWrite] = useState(false);
  const [orderActions, setOrderActions] = useState(true);
  const [rateLimit, setRateLimit] = useState("300");
  const [ips, setIps] = useState("");
  const [ttl, setTtl] = useState("24");
  const [result, setResult] = useState<GlorianConnectionPackage | null>(null);

  const reset = () => {
    setPassword("");
    setConfirm("");
    setResult(null);
  };

  const ipList = parseIpList(ips);
  const badIp = ipList.find((ip) => !isIpAddress(ip));
  const passwordError =
    password.length > 0 && password.length < MIN_PASSWORD
      ? t("integrations.glorian.package.password_short")
      : confirm.length > 0 && confirm !== password
        ? t("integrations.glorian.package.password_mismatch")
        : null;
  const canSubmit =
    baseUrl.trim() !== "" &&
    password.length >= MIN_PASSWORD &&
    confirm === password &&
    !badIp;

  const create = useMutation({
    mutationFn: () =>
      glorianService.createPackage({
        base_url: baseUrl.trim(),
        password,
        dealer_write: dealerWrite,
        order_status_actions: orderActions,
        rate_limit_per_minute: Number(rateLimit) || 300,
        ip_allowlist: ipList,
        ttl_hours: Number(ttl) || 24,
      }),
    onSuccess: async (pkg) => {
      setPassword("");
      setConfirm("");
      setResult(pkg);
      downloadPackage(pkg);
      queryClient.setQueryData(glorianKeys.connection(), pkg.connection);
      await queryClient.invalidateQueries({ queryKey: glorianKeys.all });
    },
    onError: (error) => appToast.error(glorianErrorText(t, error)),
  });

  return (
    <>
      <Button
        type="button"
        variant="outline"
        onClick={() => {
          reset();
          setBaseUrl(connection.base_url);
          setOpen(true);
        }}
        data-testid="glorian-package-open"
      >
        <FileKey2 className="size-4" aria-hidden />
        {t("integrations.glorian.package.button")}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next);
          if (!next) reset();
        }}
      >
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("integrations.glorian.package.title")}</DialogTitle>
            <DialogDescription>
              {t("integrations.glorian.package.description")}
            </DialogDescription>
          </DialogHeader>

          {result ? (
            <div className="space-y-4" data-testid="glorian-package-result">
              <Alert>
                <AlertDescription className="space-y-2">
                  <p className="font-medium">
                    {t("integrations.glorian.package.done_title")}
                  </p>
                  <p>
                    {result.rotated
                      ? t("integrations.glorian.package.done_rotated")
                      : t("integrations.glorian.package.done_first")}
                  </p>
                  <p className="font-mono text-xs" dir="ltr">
                    {t("integrations.glorian.package.fingerprint", {
                      fp: result.key_fingerprint,
                    })}
                  </p>
                  <p className="text-muted-foreground text-sm">
                    {t("integrations.glorian.package.expires", {
                      date: format.dateTime(result.expires_at),
                    })}
                  </p>
                </AlertDescription>
              </Alert>
              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => downloadPackage(result)}
                >
                  <Download className="size-4" aria-hidden />
                  {t("integrations.glorian.package.download_again")}
                </Button>
                <Button type="button" onClick={() => setOpen(false)}>
                  {t("common.close")}
                </Button>
              </DialogFooter>
            </div>
          ) : (
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault();
                if (canSubmit && !create.isPending) create.mutate();
              }}
            >
              <div className="space-y-2">
                <Label htmlFor="glorian-package-url">
                  {t("integrations.glorian.package.base_url")}
                </Label>
                <Input
                  id="glorian-package-url"
                  value={baseUrl}
                  onChange={(e) => setBaseUrl(e.target.value)}
                  placeholder="https://glorian.example.com"
                  inputMode="url"
                  autoComplete="off"
                  dir="ltr"
                />
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="glorian-package-password">
                    {t("integrations.glorian.package.password")}
                  </Label>
                  <Input
                    id="glorian-package-password"
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="new-password"
                    data-testid="glorian-package-password"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="glorian-package-confirm">
                    {t("integrations.glorian.package.password_confirm")}
                  </Label>
                  <Input
                    id="glorian-package-confirm"
                    type="password"
                    value={confirm}
                    onChange={(e) => setConfirm(e.target.value)}
                    autoComplete="new-password"
                    data-testid="glorian-package-confirm"
                  />
                </div>
                <p
                  className={
                    passwordError
                      ? "text-destructive text-sm sm:col-span-2"
                      : "text-muted-foreground text-sm sm:col-span-2"
                  }
                >
                  {passwordError ??
                    t("integrations.glorian.package.password_hint")}
                </p>
              </div>

              <ToggleRow
                id="glorian-package-dealer-write"
                label={t("integrations.glorian.package.dealer_write")}
                hint={t("integrations.glorian.package.dealer_write_hint")}
                checked={dealerWrite}
                onChange={setDealerWrite}
              />
              <ToggleRow
                id="glorian-package-order-actions"
                label={t("integrations.glorian.package.order_status_actions")}
                hint={t(
                  "integrations.glorian.package.order_status_actions_hint",
                )}
                checked={orderActions}
                onChange={setOrderActions}
              />

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="glorian-package-rate">
                    {t("integrations.glorian.package.rate_limit")}
                  </Label>
                  <Input
                    id="glorian-package-rate"
                    type="number"
                    min={1}
                    max={10000}
                    value={rateLimit}
                    onChange={(e) => setRateLimit(e.target.value)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="glorian-package-ttl">
                    {t("integrations.glorian.package.ttl_hours")}
                  </Label>
                  <Input
                    id="glorian-package-ttl"
                    type="number"
                    min={1}
                    max={72}
                    value={ttl}
                    onChange={(e) => setTtl(e.target.value)}
                  />
                </div>
                <p className="text-muted-foreground text-sm sm:col-span-2">
                  {t("integrations.glorian.package.ttl_hint")}
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="glorian-package-ips">
                  {t("integrations.glorian.package.ip_allowlist")}
                </Label>
                <Textarea
                  id="glorian-package-ips"
                  value={ips}
                  onChange={(e) => setIps(e.target.value)}
                  rows={2}
                  dir="ltr"
                  placeholder="203.0.113.10"
                />
                <p
                  className={
                    badIp
                      ? "text-destructive text-sm"
                      : "text-muted-foreground text-sm"
                  }
                >
                  {badIp
                    ? t("integrations.glorian.package.invalid_ip", {
                        ip: badIp,
                      })
                    : t("integrations.glorian.package.ip_allowlist_hint")}
                </p>
              </div>

              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setOpen(false)}
                >
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={!canSubmit || create.isPending}
                  data-testid="glorian-package-submit"
                >
                  {create.isPending
                    ? t("integrations.glorian.package.creating")
                    : t("integrations.glorian.package.submit")}
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

function ToggleRow({
  id,
  label,
  hint,
  checked,
  onChange,
}: {
  id: string;
  label: string;
  hint: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
      <div className="space-y-1">
        <Label htmlFor={id}>{label}</Label>
        <p className="text-muted-foreground text-sm">{hint}</p>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onChange} />
    </div>
  );
}
