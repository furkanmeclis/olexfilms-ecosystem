"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  CHANNELS,
  DELIVERY_STATUSES,
  LANGUAGES,
  ROLES,
  deliveryStatusVariant,
} from "@/features/notification-center/lib/options";
import {
  notificationCenterService,
  type NotificationChannel,
  type NotificationDeliveryStatus,
  type NotificationEvent,
  type NotificationRendered,
  type NotificationTemplate,
  type NotificationTemplateInput,
  type NotificationTemplateRole,
} from "@/features/notification-center/services/notification-center.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const KEY = ["platform", "notification-center"] as const;
const ALL = "__all__";
const PAGE_SIZE = 25;

function useToastError() {
  const { t } = useLocale();
  return (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("notifications.center.failed"),
    );
}

export function NotificationCenterPage() {
  const { t } = useLocale();
  return (
    <div className="space-y-6">
      <PageHeader
        icon={<BellRing className="size-7" />}
        title={t("notifications.center.title")}
        description={t("notifications.center.description")}
      />
      <Tabs defaultValue="templates">
        <TabsList>
          <TabsTrigger value="templates">
            {t("notifications.center.tabs.templates")}
          </TabsTrigger>
          <TabsTrigger value="channels">
            {t("notifications.center.tabs.channels")}
          </TabsTrigger>
          <TabsTrigger value="deliveries">
            {t("notifications.center.tabs.deliveries")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="templates" className="pt-4">
          <TemplatesPanel />
        </TabsContent>
        <TabsContent value="channels" className="pt-4">
          <ChannelsPanel />
        </TabsContent>
        <TabsContent value="deliveries" className="pt-4">
          <DeliveriesPanel />
        </TabsContent>
      </Tabs>
    </div>
  );
}

// --- Templates ----------------------------------------------------------------

function TemplatesPanel() {
  const { t } = useLocale();
  const events = useQuery({
    queryKey: [...KEY, "events"],
    queryFn: () => notificationCenterService.events(),
  });
  const [code, setCode] = useState<string>("");
  const event = events.data?.find((e) => e.code === code) ?? events.data?.[0];

  if (events.isLoading) return <Loading label={t("common.loading")} />;
  if (events.isError || !event) {
    return (
      <ErrorState
        title={t("notifications.center.error")}
        retryLabel={t("common.retry")}
        onRetry={() => events.refetch()}
      />
    );
  }
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <Label htmlFor="nc-event">{t("notifications.center.event")}</Label>
        <Select value={event.code} onValueChange={setCode}>
          <SelectTrigger id="nc-event" className="w-[320px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {events.data?.map((e) => (
              <SelectItem key={e.code} value={e.code}>
                {e.code}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {event.critical ? (
          <Badge variant="danger">{t("notifications.center.critical")}</Badge>
        ) : null}
      </div>
      <TemplateMatrix key={event.code} event={event} />
    </div>
  );
}

const emptyDraft = (
  event: NotificationEvent,
): Required<
  Pick<
    NotificationTemplateInput,
    "code" | "role" | "channel" | "language" | "subject" | "body"
  >
> => ({
  code: event.code,
  role: "generic",
  channel: event.default_channels[0] ?? "inapp",
  language: "tr",
  subject: "",
  body: "",
});

function TemplateMatrix({ event }: { event: NotificationEvent }) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const onError = useToastError();
  const templates = useQuery({
    queryKey: [...KEY, "templates", event.code],
    queryFn: () => notificationCenterService.templates(event.code),
  });
  const [draft, setDraft] = useState(() => emptyDraft(event));
  const [preview, setPreview] = useState<NotificationRendered | null>(null);

  const save = useMutation({
    mutationFn: () =>
      notificationCenterService.saveTemplate({ ...draft, active: true }),
    onSuccess: async () => {
      appToast.success(t("notifications.center.saved"));
      await queryClient.invalidateQueries({
        queryKey: [...KEY, "templates", event.code],
      });
    },
    onError,
  });
  const renderPreview = useMutation({
    mutationFn: () => notificationCenterService.preview(draft),
    onSuccess: setPreview,
    onError,
  });

  const edit = (tpl: NotificationTemplate) => {
    setPreview(null);
    setDraft({
      code: tpl.code,
      role: tpl.role,
      channel: tpl.channel,
      language: tpl.language,
      subject: tpl.subject,
      body: tpl.body,
    });
  };

  return (
    <div className="grid gap-6 xl:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>{t("notifications.center.existing")}</CardTitle>
          <CardDescription>
            {t("notifications.center.fallback_hint")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {templates.isLoading ? (
            <Loading label={t("common.loading")} />
          ) : (templates.data ?? []).length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("notifications.center.no_templates")}
            </p>
          ) : (
            <ul className="divide-y text-sm">
              {templates.data?.map((tpl) => (
                <li key={tpl.uuid}>
                  <button
                    type="button"
                    onClick={() => edit(tpl)}
                    className="hover:bg-muted flex w-full flex-wrap items-center gap-2 px-2 py-2 text-start"
                  >
                    <Badge variant="outline">
                      {t(`notifications.center.roles.${tpl.role}`)}
                    </Badge>
                    <Badge variant="secondary">
                      {t(`notifications.center.channels.${tpl.channel}`)}
                    </Badge>
                    <Badge variant="outline">{tpl.language}</Badge>
                    <span className="truncate">{tpl.subject || tpl.body}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("notifications.center.editor")}</CardTitle>
          <CardDescription>
            {t("notifications.center.placeholders")}:{" "}
            {(event.placeholders ?? []).length === 0
              ? "—"
              : (event.placeholders ?? []).map((p) => (
                  <code key={p.key} className="me-2">{`{{${p.key}}}`}</code>
                ))}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <OptionSelect
              id="nc-role"
              label={t("notifications.center.role")}
              value={draft.role}
              options={ROLES.map((r) => ({
                value: r,
                label: t(`notifications.center.roles.${r}`),
              }))}
              onChange={(role) =>
                setDraft({ ...draft, role: role as NotificationTemplateRole })
              }
            />
            <OptionSelect
              id="nc-channel"
              label={t("notifications.center.channel")}
              value={draft.channel}
              options={CHANNELS.map((c) => ({
                value: c,
                label: t(`notifications.center.channels.${c}`),
              }))}
              onChange={(channel) =>
                setDraft({ ...draft, channel: channel as NotificationChannel })
              }
            />
            <OptionSelect
              id="nc-language"
              label={t("notifications.center.language")}
              value={draft.language}
              options={LANGUAGES.map((l) => ({ value: l, label: l }))}
              onChange={(language) => setDraft({ ...draft, language })}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="nc-subject">
              {t("notifications.center.subject")}
            </Label>
            <Input
              id="nc-subject"
              value={draft.subject}
              dir="auto"
              onChange={(e) => setDraft({ ...draft, subject: e.target.value })}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="nc-body">{t("notifications.center.body")}</Label>
            <Textarea
              id="nc-body"
              rows={6}
              dir="auto"
              value={draft.body}
              onChange={(e) => setDraft({ ...draft, body: e.target.value })}
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              disabled={renderPreview.isPending || !draft.body.trim()}
              onClick={() => renderPreview.mutate()}
            >
              {t("notifications.center.preview")}
            </Button>
            <Button
              disabled={save.isPending || !draft.body.trim()}
              onClick={() => save.mutate()}
            >
              {t("common.save")}
            </Button>
          </div>
          {preview ? <PreviewBox preview={preview} /> : null}
        </CardContent>
      </Card>
    </div>
  );
}

function PreviewBox({ preview }: { preview: NotificationRendered }) {
  if (preview.html) {
    return (
      <iframe
        title="preview"
        sandbox=""
        srcDoc={preview.html}
        className="h-96 w-full rounded-md border bg-white"
      />
    );
  }
  return (
    <div dir={preview.dir} className="bg-muted rounded-md p-3 text-sm">
      {preview.subject ? (
        <p className="font-semibold">{preview.subject}</p>
      ) : null}
      <p className="whitespace-pre-wrap">{preview.body}</p>
    </div>
  );
}

function OptionSelect({
  id,
  label,
  value,
  options,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (v: string) => void;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id={id}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// --- Channels -----------------------------------------------------------------

function ChannelsPanel() {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const onError = useToastError();
  const channels = useQuery({
    queryKey: [...KEY, "channels"],
    queryFn: () => notificationCenterService.channels(),
  });
  const toggle = useMutation({
    mutationFn: ({
      channel,
      enabled,
    }: {
      channel: NotificationChannel;
      enabled: boolean;
    }) => notificationCenterService.setChannel(channel, enabled),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: [...KEY, "channels"] });
    },
    onError,
  });

  if (channels.isLoading) return <Loading label={t("common.loading")} />;
  if (channels.isError) {
    return (
      <ErrorState
        title={t("notifications.center.error")}
        retryLabel={t("common.retry")}
        onRetry={() => channels.refetch()}
      />
    );
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("notifications.center.tabs.channels")}</CardTitle>
        <CardDescription>
          {t("notifications.center.channels_hint")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {channels.data?.map((c) => (
          <div
            key={c.channel}
            className="flex items-center justify-between gap-4"
          >
            <Label htmlFor={`nc-ch-${c.channel}`}>
              {t(`notifications.center.channels.${c.channel}`)}
            </Label>
            <Switch
              id={`nc-ch-${c.channel}`}
              checked={c.enabled}
              disabled={toggle.isPending}
              onCheckedChange={(enabled) =>
                toggle.mutate({ channel: c.channel, enabled })
              }
            />
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

// --- Deliveries ---------------------------------------------------------------

function DeliveriesPanel() {
  const { t } = useLocale();
  const [status, setStatus] = useState<string>(ALL);
  const [channel, setChannel] = useState<string>(ALL);
  const [offset, setOffset] = useState(0);
  const filter = useMemo(
    () => ({
      limit: PAGE_SIZE,
      offset,
      status:
        status === ALL ? undefined : (status as NotificationDeliveryStatus),
      channel: channel === ALL ? undefined : (channel as NotificationChannel),
    }),
    [status, channel, offset],
  );
  const page = useQuery({
    queryKey: [...KEY, "deliveries", filter],
    queryFn: () => notificationCenterService.deliveries(filter),
  });
  const fmt = (v: string) => new Date(v).toLocaleString();

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("notifications.center.tabs.deliveries")}</CardTitle>
        <CardDescription>
          {t("notifications.center.deliveries_hint")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid max-w-xl gap-3 sm:grid-cols-2">
          <OptionSelect
            id="nc-f-status"
            label={t("notifications.center.status")}
            value={status}
            options={[
              { value: ALL, label: t("notifications.center.all") },
              ...DELIVERY_STATUSES.map((s) => ({
                value: s,
                label: t(`notifications.center.statuses.${s}`),
              })),
            ]}
            onChange={(v) => {
              setStatus(v);
              setOffset(0);
            }}
          />
          <OptionSelect
            id="nc-f-channel"
            label={t("notifications.center.channel")}
            value={channel}
            options={[
              { value: ALL, label: t("notifications.center.all") },
              ...CHANNELS.map((c) => ({
                value: c,
                label: t(`notifications.center.channels.${c}`),
              })),
            ]}
            onChange={(v) => {
              setChannel(v);
              setOffset(0);
            }}
          />
        </div>
        {page.isLoading ? <Loading label={t("common.loading")} /> : null}
        {page.isError ? (
          <ErrorState
            title={t("notifications.center.error")}
            retryLabel={t("common.retry")}
            onRetry={() => page.refetch()}
          />
        ) : null}
        {page.data ? (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-muted-foreground text-start">
                  <tr>
                    <th className="p-2 text-start">
                      {t("notifications.center.created")}
                    </th>
                    <th className="p-2 text-start">
                      {t("notifications.center.event")}
                    </th>
                    <th className="p-2 text-start">
                      {t("notifications.center.recipient")}
                    </th>
                    <th className="p-2 text-start">
                      {t("notifications.center.channel")}
                    </th>
                    <th className="p-2 text-start">
                      {t("notifications.center.language")}
                    </th>
                    <th className="p-2 text-start">
                      {t("notifications.center.status")}
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {page.data.items.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="text-muted-foreground p-4">
                        {t("notifications.center.no_deliveries")}
                      </td>
                    </tr>
                  ) : (
                    page.data.items.map((d) => (
                      <tr key={d.uuid}>
                        <td className="p-2 whitespace-nowrap">
                          {fmt(d.created_at)}
                        </td>
                        <td className="p-2">{d.event_code}</td>
                        <td className="p-2">{d.user_email || d.user_uuid}</td>
                        <td className="p-2">
                          {t(`notifications.center.channels.${d.channel}`)}
                        </td>
                        <td className="p-2">{d.language || "—"}</td>
                        <td className="p-2" title={d.error || undefined}>
                          <Badge variant={deliveryStatusVariant(d.status)}>
                            {t(`notifications.center.statuses.${d.status}`)}
                          </Badge>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">
                {t("notifications.center.total", { count: page.data.total })}
              </span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset === 0}
                  onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
                >
                  {t("notifications.center.prev")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset + PAGE_SIZE >= page.data.total}
                  onClick={() => setOffset(offset + PAGE_SIZE)}
                >
                  {t("notifications.center.next")}
                </Button>
              </div>
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  );
}
