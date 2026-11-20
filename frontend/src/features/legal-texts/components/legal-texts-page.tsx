"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ScrollText } from "lucide-react";
import { useMemo, useState } from "react";

import { SUPPORTED_LOCALES, localeDir, normalizeLocale } from "@/config/i18n";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Markdown } from "@/features/portal/lib/markdown";
import {
  LEGAL_TEXT_KINDS,
  legalTextsService,
  type LegalTextAdmin,
  type LegalTextKind,
} from "@/features/legal-texts/services/legal-texts.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/lib/toast";

/** The content languages of the backend (K10 + fa, TEC-1233). */
const TEXT_LOCALES = SUPPORTED_LOCALES;
const queryKey = (kind: LegalTextKind) =>
  ["platform", "legal-texts", kind] as const;

function isKind(value: string): value is LegalTextKind {
  return (LEGAL_TEXT_KINDS as readonly string[]).includes(value);
}

export function LegalTextsPage() {
  const { t } = useLocale();
  const [kind, setKind] = useState<LegalTextKind>("ai_guidelines");
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: queryKey(kind),
    queryFn: () => legalTextsService.get(kind),
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("portal.legal.title")}
        description={t("portal.legal.description")}
        icon={<ScrollText className="size-6" />}
      />
      {isLoading ? <Loading /> : null}
      {isError ? (
        <ErrorState
          title={t("portal.legal.failed")}
          onRetry={() => void refetch()}
        />
      ) : null}
      <div className="max-w-xs space-y-2">
        <Label htmlFor="legal-kind">{t("portal.legal.kind_label")}</Label>
        <Select
          value={kind}
          onValueChange={(value) => {
            if (isKind(value)) setKind(value);
          }}
        >
          <SelectTrigger id="legal-kind" data-testid="legal-kind">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {LEGAL_TEXT_KINDS.map((k) => (
              <SelectItem key={k} value={k}>
                {t(`portal.legal.kind.${k}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {data && data.kind === kind ? (
        <Editor
          key={`${kind}:${data.versions[0]?.uuid ?? "empty"}`}
          kind={kind}
          view={data}
        />
      ) : null}
    </div>
  );
}

function Editor({ kind, view }: { kind: LegalTextKind; view: LegalTextAdmin }) {
  const { t, format, locale: uiLocale } = useLocale();
  const queryClient = useQueryClient();
  const [locale, setLocale] = useState<string>("tr");
  const current = view.texts.find((x) => x.locale === locale);
  const [drafts, setDrafts] = useState<Record<string, string>>(() =>
    Object.fromEntries(view.texts.map((x) => [x.locale, x.body])),
  );
  const body = drafts[locale] ?? "";

  const names = useMemo(() => {
    try {
      return new Intl.DisplayNames([uiLocale], { type: "language" });
    } catch {
      return null;
    }
  }, [uiLocale]);

  const publish = useMutation({
    mutationFn: () => legalTextsService.publish(kind, locale, body),
    onSuccess: async (res) => {
      await queryClient.invalidateQueries({ queryKey: queryKey(kind) });
      if (res.created) {
        appToast.success(
          t("portal.legal.saved", { version: res.text.version }),
        );
      } else {
        appToast.info(t("portal.legal.unchanged"));
      }
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("portal.legal.failed"),
      ),
  });

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>{t(`portal.legal.kind.${kind}`)}</CardTitle>
          <CardDescription>
            {current
              ? t("portal.legal.current_version", { version: current.version })
              : t("portal.legal.no_version")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="legal-locale">{t("portal.legal.locale")}</Label>
            <Select value={locale} onValueChange={setLocale}>
              <SelectTrigger id="legal-locale" data-testid="legal-locale">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TEXT_LOCALES.map((l) => (
                  <SelectItem key={l} value={l}>
                    {names?.of(l) ?? l} ({l})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="legal-body">{t("portal.legal.body")}</Label>
            <Textarea
              id="legal-body"
              rows={14}
              lang={locale}
              dir={localeDir(normalizeLocale(locale) ?? "tr")}
              className="font-mono text-sm"
              value={body}
              onChange={(e) =>
                setDrafts((prev) => ({ ...prev, [locale]: e.target.value }))
              }
            />
          </div>
          <Button
            onClick={() => publish.mutate()}
            disabled={!body.trim() || publish.isPending}
          >
            {t("portal.legal.save")}
          </Button>
        </CardContent>
      </Card>
      <div className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>{t("portal.legal.preview")}</CardTitle>
          </CardHeader>
          <CardContent
            dir={localeDir(normalizeLocale(locale) ?? "tr")}
            lang={locale}
          >
            <Markdown source={body} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t("portal.legal.history")}</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-1 text-sm">
              {view.versions.map((v) => (
                <li key={v.uuid} className="flex justify-between gap-4">
                  <span>
                    {v.locale} · v{v.version}
                  </span>
                  <span className="text-muted-foreground">
                    {format.dateTime(v.created_at)}
                  </span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
