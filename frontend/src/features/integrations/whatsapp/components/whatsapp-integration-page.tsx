"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MessageCircle } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { PageHeader } from "@/components/layout/page-header";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  qrPollInterval,
  statusLabelKey,
  statusVariant,
} from "@/features/integrations/whatsapp/lib/status";
import {
  whatsappService,
  type WhatsAppOverview,
} from "@/features/integrations/whatsapp/services/whatsapp.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const QUERY_KEY = ["platform", "integrations", "whatsapp"] as const;
const KVKK_LOCALES = ["tr", "en"] as const;

function useToastError() {
  const { t } = useLocale();
  return (error: unknown) => {
    appToast.error(
      isApiError(error)
        ? error.message
        : t("integrations.whatsapp.toast.failed"),
    );
  };
}

export function WhatsAppIntegrationPage() {
  const { t } = useLocale();
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => whatsappService.overview(),
    refetchInterval: 30_000,
  });

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<MessageCircle className="size-7" />}
        title={t("integrations.whatsapp.title")}
        description={t("integrations.whatsapp.description")}
      />
      {isLoading ? <Loading label={t("common.loading")} /> : null}
      {isError ? (
        <ErrorState
          title={t("integrations.whatsapp.error.title")}
          description={t("integrations.whatsapp.error.description")}
          retryLabel={t("common.retry")}
          onRetry={() => refetch()}
        />
      ) : null}
      {data ? <WhatsAppPanels overview={data} /> : null}
    </div>
  );
}

function WhatsAppPanels({ overview }: { overview: WhatsAppOverview }) {
  const { t } = useLocale();
  return (
    <div className="space-y-6">
      {!overview.configured ? (
        <Alert>
          <AlertDescription>
            {t("integrations.whatsapp.not_configured")}
          </AlertDescription>
        </Alert>
      ) : null}
      {overview.configured && !overview.webhook_configured ? (
        <Alert>
          <AlertDescription>
            {t("integrations.whatsapp.webhook_missing")}
          </AlertDescription>
        </Alert>
      ) : null}
      <div className="grid gap-6 lg:grid-cols-2">
        <StatusCard overview={overview} />
        <ConnectCard overview={overview} />
      </div>
      <div className="grid gap-6 lg:grid-cols-2">
        <PairCard />
        <TestMessageCard />
      </div>
      <SettingsCard overview={overview} />
      <EventsCard overview={overview} />
    </div>
  );
}

function StatusCard({ overview }: { overview: WhatsAppOverview }) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const onError = useToastError();
  const [confirm, setConfirm] = useState(false);
  const logout = useMutation({
    mutationFn: () => whatsappService.logout(),
    onSuccess: async () => {
      setConfirm(false);
      await queryClient.invalidateQueries({ queryKey: QUERY_KEY });
    },
    onError,
  });
  const fmt = (v?: string) => (v ? new Date(v).toLocaleString() : "—");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {t("integrations.whatsapp.status.title")}
          <Badge variant={statusVariant(overview.status)}>
            {t(statusLabelKey(overview.status))}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2">
          <dt className="text-muted-foreground">
            {t("integrations.whatsapp.status.phone")}
          </dt>
          <dd dir="ltr" className="text-start">
            {overview.phone || "—"}
          </dd>
          <dt className="text-muted-foreground">
            {t("integrations.whatsapp.status.last_seen")}
          </dt>
          <dd>{fmt(overview.last_seen_at)}</dd>
          {overview.last_error_reason ? (
            <>
              <dt className="text-muted-foreground">
                {t("integrations.whatsapp.status.last_error")}
              </dt>
              <dd className="break-all">{overview.last_error_reason}</dd>
            </>
          ) : null}
          {overview.gateway_error ? (
            <>
              <dt className="text-muted-foreground">
                {t("integrations.whatsapp.status.gateway_error")}
              </dt>
              <dd className="break-all">{overview.gateway_error}</dd>
            </>
          ) : null}
        </dl>
        {overview.logged_in ? (
          <Button variant="destructive" onClick={() => setConfirm(true)}>
            {t("integrations.whatsapp.logout")}
          </Button>
        ) : null}
        <ConfirmDialog
          open={confirm}
          title={t("integrations.whatsapp.logout")}
          description={t("integrations.whatsapp.logout_confirm")}
          variant="destructive"
          isPending={logout.isPending}
          onConfirm={() => logout.mutate()}
          onCancel={() => setConfirm(false)}
        />
      </CardContent>
    </Card>
  );
}

function ConnectCard({ overview }: { overview: WhatsAppOverview }) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const onError = useToastError();
  const [polling, setPolling] = useState(false);

  const connect = useMutation({
    mutationFn: () => whatsappService.connect(),
    onSuccess: () => setPolling(true),
    onError,
  });
  const qr = useQuery({
    queryKey: [...QUERY_KEY, "qr"],
    queryFn: async () => {
      const res = await whatsappService.qr();
      // Linked: refresh the status card; polling stops via refetchInterval.
      if (res.connected) {
        void queryClient.invalidateQueries({
          queryKey: QUERY_KEY,
          exact: true,
        });
      }
      return res;
    },
    enabled: polling,
    retry: false,
    refetchInterval: (query) =>
      qrPollInterval(polling, Boolean(query.state.data?.connected)),
  });
  const connected = Boolean(qr.data?.connected) || overview.logged_in;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("integrations.whatsapp.connect.title")}</CardTitle>
        <CardDescription>
          {t("integrations.whatsapp.connect.hint")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {connected ? (
          <p className="text-sm text-emerald-700 dark:text-emerald-300">
            {t("integrations.whatsapp.connect.connected")}
          </p>
        ) : (
          <>
            <Button
              onClick={() => connect.mutate()}
              disabled={connect.isPending || !overview.configured}
            >
              {t("integrations.whatsapp.connect.start")}
            </Button>
            {polling && !qr.data?.qr_code ? (
              <p className="text-muted-foreground text-sm">
                {t("integrations.whatsapp.connect.waiting")}
              </p>
            ) : null}
            {qr.data?.qr_code ? (
              // eslint-disable-next-line @next/next/no-img-element -- data URL from the gateway
              <img
                src={qr.data.qr_code}
                alt={t("integrations.whatsapp.connect.qr_alt")}
                className="size-64 rounded-md border bg-white p-2"
              />
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  );
}

function PairCard() {
  const { t } = useLocale();
  const onError = useToastError();
  const [phone, setPhone] = useState("");
  const pair = useMutation({
    mutationFn: () => whatsappService.pairPhone(phone),
    onError,
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("integrations.whatsapp.pair.title")}</CardTitle>
        <CardDescription>
          {t("integrations.whatsapp.pair.hint")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-2">
          <Label htmlFor="wa-pair-phone">
            {t("integrations.whatsapp.pair.phone")}
          </Label>
          <Input
            id="wa-pair-phone"
            dir="ltr"
            inputMode="tel"
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            placeholder="+90 555 123 45 67"
          />
        </div>
        <Button
          onClick={() => pair.mutate()}
          disabled={!phone.trim() || pair.isPending}
        >
          {t("integrations.whatsapp.pair.submit")}
        </Button>
        {pair.data?.linking_code ? (
          <p className="text-sm">
            {t("integrations.whatsapp.pair.code")}:{" "}
            <span dir="ltr" className="font-mono text-lg font-semibold">
              {pair.data.linking_code}
            </span>
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}

function TestMessageCard() {
  const { t } = useLocale();
  const onError = useToastError();
  const [phone, setPhone] = useState("");
  const [body, setBody] = useState(() =>
    t("integrations.whatsapp.test.default_body"),
  );
  const send = useMutation({
    mutationFn: () => whatsappService.testMessage(phone, body),
    onSuccess: () => appToast.success(t("integrations.whatsapp.test.sent")),
    onError,
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("integrations.whatsapp.test.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-2">
          <Label htmlFor="wa-test-phone">
            {t("integrations.whatsapp.test.phone")}
          </Label>
          <Input
            id="wa-test-phone"
            dir="ltr"
            inputMode="tel"
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            placeholder="+90 555 123 45 67"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="wa-test-body">
            {t("integrations.whatsapp.test.body")}
          </Label>
          <Textarea
            id="wa-test-body"
            rows={3}
            value={body}
            onChange={(e) => setBody(e.target.value)}
          />
        </div>
        <Button
          onClick={() => send.mutate()}
          disabled={!phone.trim() || !body.trim() || send.isPending}
        >
          {t("integrations.whatsapp.test.submit")}
        </Button>
      </CardContent>
    </Card>
  );
}

function SettingsCard({ overview }: { overview: WhatsAppOverview }) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const onError = useToastError();
  const initial = () =>
    Object.fromEntries(
      KVKK_LOCALES.map((l) => [
        l,
        overview.kvkk_notices.find((n) => n.locale === l)?.body ?? "",
      ]),
    ) as Record<(typeof KVKK_LOCALES)[number], string>;
  const [smsFallback, setSmsFallback] = useState(overview.sms_fallback_enabled);
  const [kvkk, setKvkk] = useState(initial);

  const save = useMutation({
    mutationFn: () =>
      whatsappService.saveSettings({
        sms_fallback_enabled: smsFallback,
        kvkk_notices: Object.fromEntries(
          Object.entries(kvkk).filter(([, v]) => v.trim() !== ""),
        ),
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: QUERY_KEY });
      appToast.success(t("integrations.whatsapp.settings.saved"));
    },
    onError,
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("integrations.whatsapp.settings.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="wa-sms-fallback">
              {t("integrations.whatsapp.settings.sms_fallback")}
            </Label>
            <p className="text-muted-foreground text-sm">
              {t("integrations.whatsapp.settings.sms_fallback_hint")}
            </p>
          </div>
          <Switch
            id="wa-sms-fallback"
            checked={smsFallback}
            onCheckedChange={setSmsFallback}
          />
        </div>
        <div className="space-y-2">
          <Label>{t("integrations.whatsapp.settings.kvkk")}</Label>
          <p className="text-muted-foreground text-sm">
            {t("integrations.whatsapp.settings.kvkk_hint")}
          </p>
          <Tabs defaultValue="tr">
            <TabsList>
              {KVKK_LOCALES.map((l) => (
                <TabsTrigger key={l} value={l}>
                  {t(`integrations.whatsapp.settings.locale_${l}`)}
                </TabsTrigger>
              ))}
            </TabsList>
            {KVKK_LOCALES.map((l) => {
              const notice = overview.kvkk_notices.find((n) => n.locale === l);
              return (
                <TabsContent key={l} value={l} className="space-y-2">
                  <Textarea
                    rows={4}
                    maxLength={2000}
                    lang={l}
                    value={kvkk[l]}
                    onChange={(e) =>
                      setKvkk((prev) => ({ ...prev, [l]: e.target.value }))
                    }
                  />
                  {notice ? (
                    <p className="text-muted-foreground text-xs">
                      {t("integrations.whatsapp.settings.version", {
                        version: notice.version,
                      })}
                    </p>
                  ) : null}
                </TabsContent>
              );
            })}
          </Tabs>
        </div>
        <Button onClick={() => save.mutate()} disabled={save.isPending}>
          {t("integrations.whatsapp.settings.save")}
        </Button>
      </CardContent>
    </Card>
  );
}

function EventsCard({ overview }: { overview: WhatsAppOverview }) {
  const { t } = useLocale();
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("integrations.whatsapp.events.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {overview.events.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("integrations.whatsapp.events.empty")}
          </p>
        ) : (
          <ul className="divide-y text-sm">
            {overview.events.map((e, i) => (
              <li
                key={`${e.created_at}-${i}`}
                className="flex flex-wrap items-center gap-2 py-2"
              >
                <span className="text-muted-foreground">
                  {new Date(e.created_at).toLocaleString()}
                </span>
                <span className="font-medium">{e.type}</span>
                {e.reason ? (
                  <span className="text-muted-foreground break-all">
                    {e.reason}
                  </span>
                ) : null}
                {e.alarm ? (
                  <Badge variant="danger">
                    {t("integrations.whatsapp.events.alarm")}
                  </Badge>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
