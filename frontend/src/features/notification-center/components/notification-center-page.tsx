"use client";

import { useState } from "react";
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
import { DeliveriesTable } from "@/features/notification-center/components/deliveries-table";
import {
  CHANNELS,
  LANGUAGES,
  ROLES,
} from "@/features/notification-center/lib/options";
import {
  notificationCenterService,
  type NotificationChannel,
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
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("notifications.center.tabs.deliveries")}</CardTitle>
        <CardDescription>
          {t("notifications.center.deliveries_hint")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <DeliveriesTable />
      </CardContent>
    </Card>
  );
}
