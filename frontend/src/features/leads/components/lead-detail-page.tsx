"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  Flame,
  ListTodo,
  MessageSquare,
  PhoneCall,
  Send,
  StickyNote,
  UserCheck,
} from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { LeadConvertDialog } from "@/features/leads/components/lead-convert-dialog";
import {
  LeadFields,
  leadInputClass,
} from "@/features/leads/components/lead-fields";
import { LeadQuotesTab } from "@/features/leads/components/lead-quotes-tab";
import {
  LEAD_STATUSES,
  leadFormOf,
  leadName,
  leadTargetTypesFor,
  leadStatusTone,
  leadTemperatureTone,
  patchBody,
  validateLeadForm,
  type LeadFormValues,
} from "@/features/leads/lib/leads";
import { leadConverted } from "@/features/leads/lib/quotes";
import {
  leadKeys,
  leadsService,
  type Lead,
  type LeadEvent,
  type LeadStatus,
} from "@/features/leads/services/leads.service";
import { isApiError } from "@/lib/api";
import { useAuthStore } from "@/lib/auth/session-store";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

function TimelineIcon({ event }: { event: LeadEvent["event_type"] }) {
  const cls = "size-4";
  if (event === "task_created") return <ListTodo className={cls} />;
  if (event === "note") return <StickyNote className={cls} />;
  if (event === "call") return <PhoneCall className={cls} />;
  if (event === "message" || event === "quote_sent")
    return <Send className={cls} />;
  if (event === "assigned") return <UserCheck className={cls} />;
  if (event === "converted") return <CheckCircle2 className={cls} />;
  return <MessageSquare className={cls} />;
}

function Timeline({ uuid }: { uuid: string }) {
  const { t, format } = useLocale();
  const events = useQuery({
    queryKey: leadKeys.events(uuid),
    queryFn: () => leadsService.events(uuid),
  });
  const items = events.data?.items ?? [];
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("leads.timeline.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {events.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            retryLabel={t("common.retry")}
            onRetry={() => void events.refetch()}
          />
        ) : items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {events.isLoading
              ? t("leads.list.loading")
              : t("leads.timeline.empty")}
          </p>
        ) : (
          <ol className="space-y-3" data-testid="lead-timeline">
            {items.map((event) => (
              <li key={event.uuid} className="flex gap-3">
                <span className="bg-muted text-muted-foreground mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full">
                  <TimelineIcon event={event.event_type} />
                </span>
                <div className="min-w-0">
                  <div className="text-sm font-medium">
                    {t(`leads.timeline.event.${event.event_type}`)}
                  </div>
                  <div className="text-muted-foreground text-xs">
                    {format.dateTime(event.created_at)}
                  </div>
                  {event.payload?.body ? (
                    <p className="mt-1 text-sm whitespace-pre-wrap">
                      {String(event.payload.body)}
                    </p>
                  ) : null}
                </div>
              </li>
            ))}
          </ol>
        )}
      </CardContent>
    </Card>
  );
}

function NoteCard({ uuid, canWrite }: { uuid: string; canWrite: boolean }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [body, setBody] = useState("");
  const add = useMutation({
    mutationFn: () => leadsService.addNote(uuid, body.trim()),
    onSuccess: async () => {
      setBody("");
      await qc.invalidateQueries({ queryKey: leadKeys.events(uuid) });
      toast.success(t("leads.note.created"));
    },
    onError: () => toast.error(t("leads.note.error")),
  });
  if (!canWrite) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("leads.note.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (body.trim()) add.mutate();
          }}
        >
          <Textarea
            data-testid="lead-note"
            rows={3}
            value={body}
            onChange={(e) => setBody(e.target.value)}
          />
          <div className="flex justify-end">
            <Button
              type="submit"
              size="sm"
              data-testid="lead-note-submit"
              disabled={!body.trim() || add.isPending}
            >
              {t("leads.note.add")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function TaskCard({ uuid, show }: { uuid: string; show: boolean }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [title, setTitle] = useState("");
  const [due, setDue] = useState("");
  const create = useMutation({
    mutationFn: () =>
      leadsService.createTask(uuid, {
        title: title.trim(),
        due_at: due ? new Date(due).toISOString() : null,
      }),
    onSuccess: async () => {
      setTitle("");
      setDue("");
      await qc.invalidateQueries({ queryKey: leadKeys.events(uuid) });
      await qc.invalidateQueries({ queryKey: leadKeys.followUpCount });
      toast.success(t("leads.task.created"));
    },
    onError: () => toast.error(t("leads.task.error")),
  });
  if (!show) return null;
  return (
    <Card data-testid="lead-task-card">
      <CardHeader>
        <CardTitle>{t("leads.task.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-3 sm:grid-cols-[1fr_220px_auto]"
          onSubmit={(e) => {
            e.preventDefault();
            if (title.trim()) create.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="lead-task-title">
              {t("leads.task.task_title")}
            </Label>
            <input
              id="lead-task-title"
              data-testid="lead-task-title"
              className={leadInputClass}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="lead-task-due">{t("leads.task.due_at")}</Label>
            <input
              id="lead-task-due"
              data-testid="lead-task-due"
              className={leadInputClass}
              type="datetime-local"
              value={due}
              onChange={(e) => setDue(e.target.value)}
            />
          </div>
          <div className="flex items-end">
            <Button
              type="submit"
              data-testid="lead-task-submit"
              disabled={!title.trim() || create.isPending}
            >
              {t("leads.task.create")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function EditCard({
  lead,
  orgType,
}: {
  lead: Lead;
  orgType: string | undefined;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [values, setValues] = useState<LeadFormValues>(() => leadFormOf(lead));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const update = useMutation({
    mutationFn: () => leadsService.patch(lead.uuid, patchBody(lead, values)),
    onSuccess: async (updated) => {
      qc.setQueryData(leadKeys.detail(lead.uuid), updated);
      await qc.invalidateQueries({ queryKey: leadKeys.lists });
      await qc.invalidateQueries({ queryKey: leadKeys.followUpCount });
      toast.success(t("leads.form.saved"));
    },
    onError: () => toast.error(t("leads.form.error")),
  });
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    const local = validateLeadForm(values);
    if (Object.keys(local).length > 0) {
      setErrors(
        Object.fromEntries(Object.entries(local).map(([k, v]) => [k, t(v)])),
      );
      return;
    }
    setErrors({});
    update.mutate();
  };
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("leads.detail.edit")}</CardTitle>
      </CardHeader>
      <CardContent>
        <form className="space-y-6" onSubmit={onSubmit} noValidate>
          <LeadFields
            targetTypes={leadTargetTypesFor(orgType)}
            values={values}
            errors={errors}
            disabled={update.isPending}
            onChange={(p) => setValues((v) => ({ ...v, ...p }))}
          />
          <div className="flex justify-end">
            <Button
              type="submit"
              data-testid="lead-save"
              disabled={update.isPending}
            >
              {t("leads.form.save")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

export function LeadDetailPage({ slug, uuid }: { slug: string; uuid: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const orgType = useAuthStore(
    (s) => s.user?.organizations.find((o) => o.slug === slug)?.type,
  );
  const isSuperAdmin = useAuthStore((s) => Boolean(s.user?.isSuperAdmin));
  const qc = useQueryClient();
  const canRead = can(permissions.leads.read);
  const canWrite = can(permissions.leads.write);
  const canQuotes = can(permissions.quotes.read);
  const canConvertOrg = can(permissions.leads.convertOrg);
  const center = orgType === "center";
  const [convertOpen, setConvertOpen] = useState(false);
  const [lostOpen, setLostOpen] = useState(false);
  const [lostReason, setLostReason] = useState("");
  const [lostError, setLostError] = useState("");
  const detail = useQuery({
    queryKey: leadKeys.detail(uuid),
    queryFn: () => leadsService.get(uuid),
    enabled: canRead && uuid !== "",
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });
  const setStatus = useMutation({
    mutationFn: (status: LeadStatus) =>
      leadsService.setStatus(uuid, {
        status,
        lost_reason: status === "lost" ? lostReason.trim() : null,
      }),
    onSuccess: async (updated) => {
      qc.setQueryData(leadKeys.detail(uuid), updated);
      setLostOpen(false);
      setLostReason("");
      await qc.invalidateQueries({ queryKey: leadKeys.events(uuid) });
      await qc.invalidateQueries({ queryKey: leadKeys.lists });
      await qc.invalidateQueries({ queryKey: leadKeys.followUpCount });
    },
    onError: () => toast.error(t("leads.status.error")),
  });
  const assign = useMutation({
    mutationFn: (assignee_user_id: number | null) =>
      leadsService.assign(uuid, { assignee_user_id }),
    onSuccess: async (updated) => {
      qc.setQueryData(leadKeys.detail(uuid), updated);
      await qc.invalidateQueries({ queryKey: leadKeys.lists });
    },
  });

  const lead = detail.data;
  const title = lead ? leadName(lead) : t("leads.detail.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Flame className="size-6" />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("leads.list.title"), href: routes.tenant.leads.list(slug) },
        { label: title },
      ]}
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("leads.list.forbidden")}
        />
      </div>
    );
  }
  if (detail.isError) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_generic")}
          retryLabel={t("common.retry")}
          onRetry={() => void detail.refetch()}
        />
      </div>
    );
  }
  if (!lead) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">
          {t("leads.list.loading")}
        </p>
      </div>
    );
  }

  const changeStatus = (status: LeadStatus) => {
    // Won goes through the conversion (TEC-316): it sets the status.
    if (status === "won") {
      setConvertOpen(true);
      return;
    }
    if (status === "lost") {
      setLostOpen(true);
      return;
    }
    setStatus.mutate(status);
  };

  return (
    <div className="space-y-6" data-testid="lead-detail">
      {header}
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <CardTitle>{t("leads.detail.summary")}</CardTitle>
          <div className="flex gap-2">
            <StatusChip
              label={t(`leads.temperature.${lead.temperature}`)}
              tone={leadTemperatureTone(lead.temperature)}
            />
            <StatusChip
              label={t(`leads.status.${lead.status}`)}
              tone={leadStatusTone(lead.status)}
            />
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label={t("leads.columns.target_type")}>
              {t(`leads.target_type.${lead.target_type}`)}
            </Field>
            <Field label={t("leads.columns.source")}>
              {t(`leads.source.${lead.source}`)}
            </Field>
            <Field label={t("leads.columns.follow_up")}>
              {lead.follow_up_date ? format.dateTime(lead.follow_up_date) : "—"}
            </Field>
            <Field label={t("leads.columns.assignee")}>
              {lead.assignee_user_id
                ? `#${lead.assignee_user_id}`
                : t("leads.form.unassigned")}
            </Field>
            <Field label={t("leads.detail.created_at")}>
              {format.dateTime(lead.created_at)}
            </Field>
            <Field label={t("leads.detail.lost_reason")}>
              {lead.lost_reason ?? "—"}
            </Field>
          </dl>
          {canWrite ? (
            <div
              className="flex flex-wrap gap-2"
              data-testid="lead-status-actions"
            >
              {LEAD_STATUSES.map((status) => (
                <Button
                  key={status}
                  type="button"
                  size="sm"
                  variant={status === "lost" ? "destructive" : "outline"}
                  data-status={status}
                  data-testid={
                    status === "won" ? "lead-convert-open" : undefined
                  }
                  disabled={
                    setStatus.isPending ||
                    status === lead.status ||
                    (status === "won" && leadConverted(lead))
                  }
                  onClick={() => changeStatus(status)}
                >
                  {status === "won"
                    ? t("leads.convert.open")
                    : t(`leads.action.${status}`)}
                </Button>
              ))}
            </div>
          ) : null}
          {canWrite ? (
            <div className="grid max-w-md gap-2 sm:grid-cols-[1fr_auto]">
              <input
                data-testid="lead-assign-input"
                className={leadInputClass}
                type="number"
                placeholder={t("leads.form.assignee_user_id")}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    assign.mutate(Number(e.currentTarget.value) || null);
                  }
                }}
              />
              <Button
                type="button"
                variant="outline"
                onClick={() => assign.mutate(null)}
              >
                {t("leads.form.unassigned")}
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>

      {lostOpen ? (
        <Card data-testid="lead-lost-dialog">
          <CardHeader>
            <CardTitle>{t("leads.lost.title")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <Textarea
              data-testid="lead-lost-reason"
              value={lostReason}
              onChange={(e) => {
                setLostReason(e.target.value);
                setLostError("");
              }}
            />
            {lostError ? (
              <p className="text-destructive text-xs" data-error="lost_reason">
                {lostError}
              </p>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setLostOpen(false)}
              >
                {t("leads.form.cancel")}
              </Button>
              <Button
                type="button"
                variant="destructive"
                data-testid="lead-lost-confirm"
                onClick={() => {
                  if (!lostReason.trim()) {
                    setLostError(t("leads.form.errors.lost_reason_required"));
                    return;
                  }
                  setStatus.mutate("lost");
                }}
              >
                {t("leads.lost.confirm")}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {canWrite ? (
        <LeadConvertDialog
          key={convertOpen ? "open" : "closed"}
          lead={lead}
          slug={slug}
          ctx={{ orgType, isSuperAdmin, canConvertOrg }}
          open={convertOpen}
          onOpenChange={setConvertOpen}
        />
      ) : null}

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview" data-testid="lead-tab-overview">
            {t("leads.detail.tab_overview")}
          </TabsTrigger>
          {canQuotes ? (
            <TabsTrigger value="quotes" data-testid="lead-tab-quotes">
              {t("leads.quotes.title")}
            </TabsTrigger>
          ) : null}
        </TabsList>
        <TabsContent value="overview" className="space-y-6">
          {canWrite ? <EditCard lead={lead} orgType={orgType} /> : null}
          <NoteCard uuid={lead.uuid} canWrite={canWrite} />
          <TaskCard uuid={lead.uuid} show={canWrite && center} />
          <Timeline uuid={lead.uuid} />
        </TabsContent>
        {canQuotes ? (
          <TabsContent value="quotes">
            <LeadQuotesTab leadUuid={lead.uuid} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
