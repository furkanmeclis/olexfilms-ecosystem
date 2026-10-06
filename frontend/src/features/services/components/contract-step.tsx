"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  FileSignature,
  ImageIcon,
  ImagePlus,
  Info,
  Loader2,
  Send,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import {
  useEffect,
  useId,
  useRef,
  useState,
  type ChangeEvent,
  type ReactNode,
} from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { localeDir, normalizeLocale } from "@/config/i18n";
import { permissions } from "@/config/permissions";
import { SignatureCanvas } from "@/features/contracts/components/signature-canvas";
import {
  formatCountdown,
  otpRequestError,
  signRequestError,
  type SignError,
} from "@/features/contracts/lib/signing";
import {
  CONTRACT_MEDIA_TYPES,
  CONTRACT_SIGN_WINDOW_MS,
  contractSigningKeys,
  contractSigningService,
  mediaFileError,
  pngBase64,
  signerOf,
  type Contract,
  type ContractMedia,
} from "@/features/contracts/services/contract-signing.service";
import { contractBlocksNext } from "@/features/services/lib/wizard";
import {
  serviceWizardKeys,
  type Service,
} from "@/features/services/services/service-wizard.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const T = "services.contract";

function FieldError({ id, error }: { id: string; error: string | null }) {
  if (!error) return null;
  return (
    <p id={id} role="alert" className="text-destructive text-sm">
      {error}
    </p>
  );
}

/**
 * The customer signing window: `start()` opens 30 minutes from now and
 * `remaining` ticks every second (null before the first OTP).
 */
function useSignWindow() {
  const [win, setWin] = useState<{ deadline: number; now: number } | null>(
    null,
  );
  const deadline = win?.deadline ?? null;
  useEffect(() => {
    if (deadline === null) return;
    const id = window.setInterval(
      () => setWin((w) => (w ? { ...w, now: Date.now() } : w)),
      1000,
    );
    return () => window.clearInterval(id);
  }, [deadline]);
  const start = () => {
    const now = Date.now();
    setWin({ deadline: now + CONTRACT_SIGN_WINDOW_MS, now });
  };
  return {
    remaining: win ? Math.max(0, win.deadline - win.now) : null,
    start,
  };
}

function SignedLine({ testId }: { testId: string }) {
  const { t } = useLocale();
  return (
    <p
      className="flex items-center gap-2 text-sm text-emerald-700 dark:text-emerald-400"
      data-testid={testId}
    >
      <CheckCircle2 className="size-4" />
      {t(`${T}.signed`)}
    </p>
  );
}

/** Customer slot: KVKK notice, OTP (30 min window), code and signature. */
function CustomerSign({
  contract,
  onSigned,
}: {
  contract: Contract;
  onSigned: (c: Contract) => void;
}) {
  const { t } = useLocale();
  const id = useId();
  const signer = signerOf(contract, "customer");
  const { remaining, start } = useSignWindow();
  const [code, setCode] = useState("");
  const [signature, setSignature] = useState<string | null>(null);
  const [error, setError] = useState<SignError | null>(null);

  const otp = useMutation({
    mutationFn: () => contractSigningService.requestCustomerOtp(contract.uuid),
    onSuccess: () => {
      setError(null);
      setCode("");
      start();
      appToast.success(t(`${T}.otp_sent`));
    },
    onError: (e: unknown) => {
      const mapped = otpRequestError(e);
      setError(mapped);
      if (!mapped.field) appToast.error(t(mapped.key));
    },
  });
  const sign = useMutation({
    mutationFn: () =>
      contractSigningService.signCustomer(contract.uuid, {
        ...(contract.otp_required ? { code: code.trim() } : {}),
        ...(signature ? { signature_png: pngBase64(signature) } : {}),
      }),
    onSuccess: (saved) => {
      setError(null);
      appToast.success(t(`${T}.customer_signed`));
      onSigned(saved);
    },
    onError: (e: unknown) => {
      const mapped = signRequestError(e);
      setError(mapped);
      if (!mapped.field) appToast.error(t(mapped.key));
    },
  });

  if (signer?.signed_at) return <SignedLine testId="customer-signed" />;

  const expired = remaining === 0;
  const errorAt = (field: SignError["field"]) =>
    error && error.field === field ? t(error.key) : null;
  const canSign =
    (!contract.signature_required || signature !== null) &&
    (!contract.otp_required || (code.trim() !== "" && !expired)) &&
    !sign.isPending;

  return (
    <div className="space-y-4" data-testid="customer-sign">
      {contract.otp_required ? (
        <div className="space-y-3">
          <Alert data-testid="kvkk-notice">
            <ShieldCheck />
            <AlertDescription>{t(`${T}.kvkk_notice`)}</AlertDescription>
          </Alert>
          <div className="flex flex-wrap items-center gap-3">
            <Button
              type="button"
              variant="outline"
              disabled={otp.isPending}
              onClick={() => otp.mutate()}
              aria-describedby={errorAt("otp") ? `${id}-otp-err` : undefined}
              data-testid="otp-send"
            >
              {otp.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Send className="size-4" />
              )}
              {remaining === null ? t(`${T}.otp_send`) : t(`${T}.otp_resend`)}
            </Button>
            {signer?.phone_e164 ? (
              <span className="text-muted-foreground text-sm" dir="ltr">
                {signer.phone_e164}
              </span>
            ) : null}
            {remaining !== null ? (
              <span
                className="text-sm tabular-nums"
                aria-live="polite"
                data-testid="otp-countdown"
              >
                {expired
                  ? t(`${T}.window_expired`)
                  : t(`${T}.window_left`, { time: formatCountdown(remaining) })}
              </span>
            ) : null}
          </div>
          <FieldError id={`${id}-otp-err`} error={errorAt("otp")} />
          <div className="max-w-xs space-y-2">
            <Label htmlFor={`${id}-code`}>{t(`${T}.code`)}</Label>
            <Input
              id={`${id}-code`}
              name="code"
              dir="ltr"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={8}
              value={code}
              aria-invalid={errorAt("code") ? true : undefined}
              aria-describedby={errorAt("code") ? `${id}-code-err` : undefined}
              onChange={(e) => {
                setCode(e.target.value);
                if (error?.field === "code") setError(null);
              }}
            />
            <FieldError id={`${id}-code-err`} error={errorAt("code")} />
          </div>
        </div>
      ) : null}
      <div className="space-y-2">
        <Label>{t(`${T}.customer_signature`)}</Label>
        <SignatureCanvas
          label={t(`${T}.customer_signature`)}
          testId="customer-canvas"
          onChange={(v) => {
            setSignature(v);
            if (error?.field === "signature") setError(null);
          }}
        />
        <FieldError id={`${id}-sig-err`} error={errorAt("signature")} />
      </div>
      {error && error.field === null ? (
        <p role="alert" className="text-destructive text-sm">
          {t(error.key)}
        </p>
      ) : null}
      <Button
        type="button"
        disabled={!canSign}
        onClick={() => sign.mutate()}
        data-testid="customer-sign-submit"
      >
        {sign.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
        {t(`${T}.sign`)}
      </Button>
    </div>
  );
}

/** Staff slot: the signed-in user's canvas signature, no OTP. */
function StaffSign({
  contract,
  onSigned,
}: {
  contract: Contract;
  onSigned: (c: Contract) => void;
}) {
  const { t } = useLocale();
  const signer = signerOf(contract, "staff");
  const [signature, setSignature] = useState<string | null>(null);
  const [error, setError] = useState<SignError | null>(null);
  const sign = useMutation({
    mutationFn: () =>
      contractSigningService.signStaff(
        contract.uuid,
        signature ? { signature_png: pngBase64(signature) } : {},
      ),
    onSuccess: (saved) => {
      setError(null);
      appToast.success(t(`${T}.staff_signed`));
      onSigned(saved);
    },
    onError: (e: unknown) => {
      const mapped = signRequestError(e);
      setError(mapped);
      if (!mapped.field) appToast.error(t(mapped.key));
    },
  });

  if (signer?.signed_at) return <SignedLine testId="staff-signed" />;

  const canSign =
    (!contract.signature_required || signature !== null) && !sign.isPending;
  return (
    <div className="space-y-3" data-testid="staff-sign">
      <Label>{t(`${T}.staff_signature`)}</Label>
      <SignatureCanvas
        label={t(`${T}.staff_signature`)}
        testId="staff-canvas"
        onChange={setSignature}
      />
      {error ? (
        <p role="alert" className="text-destructive text-sm">
          {t(error.key)}
        </p>
      ) : null}
      <Button
        type="button"
        disabled={!canSign}
        onClick={() => sign.mutate()}
        data-testid="staff-sign-submit"
      >
        {sign.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
        {t(`${T}.sign`)}
      </Button>
    </div>
  );
}

/**
 * Contract images (≤ 12 MB, jpeg/png/webp). The API keeps only a storage
 * key, so a preview is shown for files picked in this session; deleting is
 * possible until the contract is executed.
 */
function MediaPanel({
  contract,
  canWrite,
  onChanged,
}: {
  contract: Contract;
  canWrite: boolean;
  onChanged: () => void;
}) {
  const { t } = useLocale();
  const inputRef = useRef<HTMLInputElement>(null);
  const [previews, setPreviews] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const editable =
    canWrite && contract.status !== "executed" && contract.status !== "voided";

  useEffect(
    () => () => {
      for (const url of Object.values(previews)) URL.revokeObjectURL(url);
    },
    // Revoke on unmount only; later previews are added, never replaced.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const add = useMutation({
    mutationFn: (file: File) =>
      contractSigningService.addMedia(contract.uuid, file),
    onSuccess: (media, file) => {
      setPreviews((p) => ({ ...p, [media.uuid]: URL.createObjectURL(file) }));
      appToast.success(t(`${T}.media_added`));
      onChanged();
    },
    onError: () => setError(t(`${T}.errors.media_failed`)),
  });
  const remove = useMutation({
    mutationFn: (media: string) =>
      contractSigningService.deleteMedia(contract.uuid, media),
    onSuccess: () => {
      appToast.success(t(`${T}.media_removed`));
      onChanged();
    },
    onError: () => appToast.error(t(`${T}.errors.media_failed`)),
  });

  const onPick = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    const bad = mediaFileError(file);
    if (bad) {
      setError(t(`${T}.errors.media_${bad}`));
      return;
    }
    setError(null);
    add.mutate(file);
  };

  const items: ContractMedia[] = contract.media;
  return (
    <div className="space-y-3" data-testid="contract-media">
      {items.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t(`${T}.media_empty`)}</p>
      ) : (
        <ul className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {items.map((m) => (
            <li
              key={m.uuid}
              className="relative overflow-hidden rounded-md border"
              data-testid="contract-media-item"
            >
              {previews[m.uuid] ? (
                // eslint-disable-next-line @next/next/no-img-element -- local blob preview
                <img
                  src={previews[m.uuid]}
                  alt={m.title ?? t(`${T}.media`)}
                  className="aspect-square w-full object-cover"
                />
              ) : (
                <div className="bg-muted text-muted-foreground flex aspect-square w-full items-center justify-center">
                  <ImageIcon className="size-8" />
                </div>
              )}
              <div className="flex items-center justify-between gap-1 p-1.5 text-xs">
                <span className="truncate" dir="ltr">
                  {(m.size_bytes / (1024 * 1024)).toFixed(1)} MB
                </span>
                {editable ? (
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="size-7"
                    aria-label={t(`${T}.media_remove`)}
                    disabled={remove.isPending}
                    onClick={() => remove.mutate(m.uuid)}
                    data-testid="contract-media-remove"
                  >
                    <Trash2 className="size-4" />
                  </Button>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      )}
      {editable ? (
        <div className="space-y-1">
          <input
            ref={inputRef}
            type="file"
            accept={CONTRACT_MEDIA_TYPES.join(",")}
            className="hidden"
            onChange={onPick}
            data-testid="contract-media-input"
          />
          <Button
            type="button"
            variant="outline"
            disabled={add.isPending}
            onClick={() => inputRef.current?.click()}
          >
            {add.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <ImagePlus className="size-4" />
            )}
            {t(`${T}.media_add`)}
          </Button>
          <p className="text-muted-foreground text-xs">
            {t(`${T}.media_hint`)}
          </p>
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3 rounded-lg border p-4">
      <h3 className="font-medium">{title}</h3>
      {children}
    </section>
  );
}

export type ContractStepProps = {
  service: Service;
  onBack: () => void;
  onNext: () => void;
};

/**
 * Wizard step "Sözleşme" (TEC-291): creates the intake contract of the
 * draft, shows its rendered content, collects the customer (OTP + canvas)
 * and staff signatures and the images. With `contract_required` the next
 * step stays closed until the contract is executed.
 */
export function ContractStep({ service, onBack, onNext }: ContractStepProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const canRead = can(permissions.contracts.read);
  const canWrite = can(permissions.contracts.write);
  // The service summary follows the cache; `created` covers the moment
  // between creating the contract and the service refetch.
  const [created, setCreated] = useState<Contract | null>(null);
  const linked =
    service.contract && service.contract.status !== "voided"
      ? service.contract
      : created;

  const contract = useQuery({
    queryKey: contractSigningKeys.detail(linked?.uuid ?? ""),
    queryFn: () => contractSigningService.get(linked?.uuid ?? ""),
    enabled: Boolean(linked) && canRead,
  });

  const stored = (c: Contract) => {
    queryClient.setQueryData(contractSigningKeys.detail(c.uuid), c);
    queryClient.setQueryData<Service>(
      serviceWizardKeys.service(service.uuid),
      (old) =>
        old
          ? {
              ...old,
              contract: {
                uuid: c.uuid,
                status: c.status,
                contract_no: c.contract_no,
                pdf_ready: c.pdf_ready,
              },
            }
          : old,
    );
  };

  const create = useMutation({
    mutationFn: () => contractSigningService.createForService(service.uuid),
    onSuccess: (c) => {
      setCreated(c);
      stored(c);
      appToast.success(t(`${T}.created`, { no: c.contract_no }));
    },
    onError: () => appToast.error(t(`${T}.errors.create_failed`)),
  });

  const data = contract.data;
  const status = data?.status ?? linked?.status;
  const blocked = contractBlocksNext({
    contract_required: service.contract_required,
    contract: status ? { status } : null,
  });

  let body;
  if (!canRead) {
    body = (
      <Alert>
        <Info />
        <AlertDescription>{t(`${T}.forbidden`)}</AlertDescription>
      </Alert>
    );
  } else if (!linked) {
    body = (
      <div
        className="flex flex-col items-start gap-3"
        data-testid="no-contract"
      >
        <p className="text-muted-foreground text-sm">
          {service.contract?.status === "voided"
            ? t(`${T}.voided_hint`)
            : t(`${T}.none`)}
        </p>
        {canWrite ? (
          <Button
            type="button"
            disabled={create.isPending}
            onClick={() => create.mutate()}
            data-testid="contract-create"
          >
            {create.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <FileSignature className="size-4" />
            )}
            {t(`${T}.create`)}
          </Button>
        ) : null}
      </div>
    );
  } else if (contract.isLoading) {
    body = <Loading />;
  } else if (contract.isError || !data) {
    body = (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void contract.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  } else {
    const locale = normalizeLocale(data.locale) ?? "tr";
    const executed = data.status === "executed";
    const signable = canWrite && data.status === "pending";
    const refresh = () =>
      void queryClient.invalidateQueries({
        queryKey: contractSigningKeys.detail(data.uuid),
      });
    body = (
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="secondary" data-testid="contract-no">
            <span dir="ltr">#{data.contract_no}</span>
          </Badge>
          <Badge
            variant={executed ? "default" : "outline"}
            data-testid="contract-status"
          >
            {t(`${T}.status.${data.status}`)}
          </Badge>
        </div>
        {executed ? (
          <Alert data-testid="contract-executed">
            <CheckCircle2 />
            <AlertDescription>{t(`${T}.executed_hint`)}</AlertDescription>
          </Alert>
        ) : null}
        <Section title={t(`${T}.preview`)}>
          <iframe
            title={t(`${T}.preview`)}
            sandbox=""
            className="h-80 w-full rounded-md border bg-white"
            data-testid="contract-preview"
            srcDoc={`<!doctype html><html dir="${localeDir(locale)}" lang="${locale}"><head><meta charset="utf-8"></head><body style="font-family:system-ui,sans-serif;padding:16px">${data.rendered_html ?? ""}</body></html>`}
          />
        </Section>
        {signable || executed ? (
          <div className="grid gap-4 lg:grid-cols-2">
            <Section title={t(`${T}.customer`)}>
              <p className="text-sm">{signerOf(data, "customer")?.name}</p>
              <CustomerSign contract={data} onSigned={stored} />
            </Section>
            <Section title={t(`${T}.staff`)}>
              <p className="text-sm">{signerOf(data, "staff")?.name}</p>
              <StaffSign contract={data} onSigned={stored} />
            </Section>
          </div>
        ) : null}
        <Section title={t(`${T}.media`)}>
          <MediaPanel contract={data} canWrite={canWrite} onChanged={refresh} />
        </Section>
      </div>
    );
  }

  return (
    <div className="space-y-6" data-testid="contract-step">
      {service.contract_required ? (
        <Alert data-testid="contract-required">
          <Info />
          <AlertDescription>{t(`${T}.required_hint`)}</AlertDescription>
        </Alert>
      ) : null}
      {body}
      <div className="flex flex-wrap justify-between gap-2">
        <Button type="button" variant="outline" onClick={onBack}>
          {t("services.wizard.back")}
        </Button>
        <Button
          type="button"
          disabled={blocked}
          onClick={onNext}
          data-testid="contract-next"
        >
          {status === "executed" || service.contract_required
            ? t("services.wizard.next")
            : t(`${T}.skip`)}
        </Button>
      </div>
    </div>
  );
}
