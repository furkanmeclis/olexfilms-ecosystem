"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ScrollText } from "lucide-react";
import { useMemo, useState } from "react";

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
  legalTextsService,
  type LegalTextAdmin,
  type LegalTextKind,
} from "@/features/legal-texts/services/legal-texts.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** The 13 content languages of the backend (K10). */
const TEXT_LOCALES = [
  "tr",
  "en",
  "bg",
  "de",
  "el",
  "uk",
  "ru",
  "fr",
  "es",
  "it",
  "zh-CN",
  "az",
  "ar",
] as const;
const RTL = new Set(["ar"]);
const KIND: LegalTextKind = "ai_guidelines";
const QUERY_KEY = ["platform", "legal-texts", KIND] as const;

export function LegalTextsPage() {
  const { t } = useLocale();
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => legalTextsService.get(KIND),
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
      {data ? (
        <Editor key={data.versions[0]?.uuid ?? "empty"} view={data} />
      ) : null}
    </div>
  );
}

function Editor({ view }: { view: LegalTextAdmin }) {
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
    mutationFn: () => legalTextsService.publish(KIND, locale, body),
    onSuccess: async (res) => {
      await queryClient.invalidateQueries({ queryKey: QUERY_KEY });
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
          <CardTitle>{t("portal.legal.kind.ai_guidelines")}</CardTitle>
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
              dir={RTL.has(locale) ? "rtl" : "ltr"}
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
          <CardContent dir={RTL.has(locale) ? "rtl" : "ltr"} lang={locale}>
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
