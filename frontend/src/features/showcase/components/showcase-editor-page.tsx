"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowUp,
  Eye,
  ImagePlus,
  Loader2,
  Plus,
  Save,
  Send,
  Trash2,
} from "lucide-react";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { FeatureGuard } from "@/features/modules/components/feature-guard";
import { ShowcasePreview } from "@/features/showcase/components/showcase-preview";
import {
  showcaseKeys,
  showcaseService,
  type Showcase,
  type ShowcaseInput,
  type ShowcaseService,
} from "@/features/showcase/services/showcase.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const LOCALES = [
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

const DAYS = [
  "monday",
  "tuesday",
  "wednesday",
  "thursday",
  "friday",
  "saturday",
  "sunday",
] as const;

const SOCIALS = ["website", "instagram", "facebook", "youtube", "tiktok"];
const MAX_PHOTO_BYTES = 5 * 1024 * 1024;

type LocaleCode = (typeof LOCALES)[number];
type ContentDraft = Record<string, { headline: string; about: string }>;

function normalizeLocale(locale: string): LocaleCode {
  return locale === "zh_CN" ? "zh-CN" : (locale as LocaleCode);
}

function initialContent(showcase: Showcase): ContentDraft {
  return Object.fromEntries(
    LOCALES.map((locale) => [
      locale,
      {
        headline: showcase.content[locale]?.headline ?? "",
        about: showcase.content[locale]?.about ?? "",
      },
    ]),
  );
}

function statusTone(status: Showcase["status"]) {
  if (status === "published") return "text-emerald-700";
  if (status === "pending_review") return "text-amber-700";
  if (status === "rejected") return "text-destructive";
  return "text-muted-foreground";
}

function move<T>(items: T[], index: number, direction: -1 | 1) {
  const next = [...items];
  const target = index + direction;
  if (target < 0 || target >= next.length) return next;
  [next[index], next[target]] = [next[target], next[index]];
  return next;
}

function compactContent(content: ContentDraft): ShowcaseInput["content"] {
  return Object.fromEntries(
    Object.entries(content)
      .map(([locale, value]) => [
        locale,
        {
          headline: value.headline.trim(),
          about: value.about.trim(),
        },
      ])
      .filter(([, value]) => {
        const v = value as { headline: string; about: string };
        return v.headline || v.about;
      }),
  );
}

function EditorInner({ slug, orgUuid }: { slug: string; orgUuid?: string }) {
  const { t, locale, format } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const target = orgUuid ? { org: orgUuid } : undefined;
  const orgLocale = normalizeLocale(locale);
  const [activeLocale, setActiveLocale] = useState<LocaleCode>(orgLocale);
  const [content, setContent] = useState<ContentDraft | null>(null);
  const [socialLinks, setSocialLinks] = useState<Record<string, string>>({});
  const [keywords, setKeywords] = useState("");
  const [placeId, setPlaceId] = useState("");
  const [workingHours, setWorkingHours] = useState<Showcase["working_hours"]>(
    {},
  );
  const [rating, setRating] = useState("");
  const [reviewCount, setReviewCount] = useState("");
  const [serviceTitle, setServiceTitle] = useState("");
  const [serviceDescription, setServiceDescription] = useState("");
  const [serviceVisible, setServiceVisible] = useState(true);
  const [photoCaption, setPhotoCaption] = useState("");
  const [photoError, setPhotoError] = useState<string | null>(null);

  const query = useQuery({
    queryKey: showcaseKeys.editor(orgUuid),
    queryFn: () => showcaseService.get(target),
  });

  const showcase = query.data;
  useEffect(() => {
    if (!showcase || content) return;
    // The editor owns a draft copy of the loaded showcase until Save is pressed.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setContent(initialContent(showcase));
    setSocialLinks(showcase.social_links);
    setKeywords(showcase.seo_keywords.join(", "));
    setPlaceId(
      showcase.google_place_id ?? showcase.google_place_id_suggestion ?? "",
    );
    setWorkingHours(showcase.working_hours);
    setRating(showcase.google_rating?.toString() ?? "");
    setReviewCount(showcase.google_review_count?.toString() ?? "");
  }, [content, showcase]);

  const refresh = () =>
    void qc.invalidateQueries({ queryKey: showcaseKeys.all });
  const error = (err: unknown) =>
    appToast.error(isApiError(err) ? err.message : t("common.error_generic"));

  const save = useMutation({
    mutationFn: async () => {
      if (!content) throw new Error("content");
      const body: ShowcaseInput = {
        content: compactContent(content),
        working_hours: workingHours,
        social_links: socialLinks,
        seo_keywords: keywords
          .split(",")
          .map((x) => x.trim())
          .filter(Boolean),
        google_place_id: placeId.trim() || null,
      };
      return showcaseService.save(body, target);
    },
    onSuccess: () => {
      appToast.success(t("showcase.toast.saved"));
      refresh();
    },
    onError: error,
  });

  const submit = useMutation({
    mutationFn: () => showcaseService.submit(target),
    onSuccess: (next) => {
      appToast.success(
        next.approval_required
          ? t("showcase.toast.submitted")
          : t("showcase.toast.published"),
      );
      refresh();
    },
    onError: error,
  });

  const saveRating = useMutation({
    mutationFn: () =>
      showcaseService.setGoogleRating(
        {
          rating: rating.trim() ? Number(rating) : null,
          review_count: reviewCount.trim() ? Number(reviewCount) : null,
        },
        target,
      ),
    onSuccess: () => {
      appToast.success(t("showcase.toast.rating_saved"));
      refresh();
    },
    onError: error,
  });

  const addService = useMutation({
    mutationFn: () =>
      showcaseService.createService(
        {
          kind: "custom",
          visible: serviceVisible,
          title: { [activeLocale]: serviceTitle },
          description: { [activeLocale]: serviceDescription },
        },
        target,
      ),
    onSuccess: () => {
      setServiceTitle("");
      setServiceDescription("");
      refresh();
    },
    onError: error,
  });

  const reorderServices = useMutation({
    mutationFn: (uuids: string[]) =>
      showcaseService.reorderServices(uuids, target),
    onSuccess: refresh,
    onError: error,
  });

  const removeService = useMutation({
    mutationFn: (uuid: string) => showcaseService.deleteService(uuid, target),
    onSuccess: refresh,
    onError: error,
  });

  const uploadPhoto = useMutation({
    mutationFn: (file: File) =>
      showcaseService.uploadPhoto(
        file,
        { [activeLocale]: photoCaption },
        target,
      ),
    onSuccess: () => {
      setPhotoCaption("");
      setPhotoError(null);
      refresh();
    },
    onError: error,
  });

  const reorderPhotos = useMutation({
    mutationFn: (uuids: string[]) =>
      showcaseService.reorderPhotos(uuids, target),
    onSuccess: refresh,
    onError: error,
  });

  const removePhoto = useMutation({
    mutationFn: (uuid: string) => showcaseService.deletePhoto(uuid, target),
    onSuccess: refresh,
    onError: error,
  });

  if (query.isLoading) return <Loading label={t("common.loading")} />;
  if (query.isError || !showcase || !content) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        description={t("showcase.editor.load_error")}
      />
    );
  }

  const submitLabel = showcase.approval_required
    ? t("showcase.actions.submit_review")
    : t("showcase.actions.publish");
  const photoLimit = showcase.max_photos || 12;
  const photoUploadDisabled = showcase.photos.length >= photoLimit;
  const canWrite = can(permissions.showcase.write);
  const suggestions = [
    showcase.organization.city,
    "PPF",
    t("showcase.suggestions.ppf"),
    t("showcase.suggestions.paint_protection"),
    t("showcase.suggestions.window_film"),
  ].filter(Boolean);

  return (
    <EntityPage
      title={t("showcase.title")}
      description={t("showcase.description")}
      permission={permissions.showcase.read}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("showcase.title") },
      ]}
      actions={
        <div className="flex flex-wrap gap-2">
          <Dialog>
            <DialogTrigger asChild>
              <Button variant="outline">
                <Eye className="size-4" />
                {t("showcase.actions.preview")}
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-5xl">
              <DialogHeader>
                <DialogTitle>{t("showcase.preview.title")}</DialogTitle>
              </DialogHeader>
              <ShowcasePreview showcase={showcase} draft />
            </DialogContent>
          </Dialog>
          <Button
            onClick={() => save.mutate()}
            disabled={!canWrite || save.isPending}
            variant="outline"
          >
            <Save className="size-4" />
            {t("showcase.actions.save")}
          </Button>
          <Button
            onClick={() => submit.mutate()}
            disabled={!canWrite || submit.isPending}
          >
            <Send className="size-4" />
            {submitLabel}
          </Button>
        </div>
      }
    >
      <div className="space-y-4">
        <Card>
          <CardContent className="flex flex-wrap items-center gap-3 pt-6">
            <span className={statusTone(showcase.status)}>
              {t(`showcase.status.${showcase.status}`)}
            </span>
            {showcase.review_note ? (
              <span className="text-muted-foreground text-sm">
                {t("showcase.status.review_note")}: {showcase.review_note}
              </span>
            ) : null}
            {showcase.updated_at ? (
              <span className="text-muted-foreground text-xs">
                {format.dateTime(showcase.updated_at)}
              </span>
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("showcase.editor.content")}</CardTitle>
          </CardHeader>
          <CardContent>
            <Tabs
              value={activeLocale}
              onValueChange={(v) => setActiveLocale(v as LocaleCode)}
            >
              <TabsList className="flex h-auto flex-wrap justify-start">
                {LOCALES.map((l) => (
                  <TabsTrigger key={l} value={l}>
                    {l}
                    {l === orgLocale ? "*" : ""}
                  </TabsTrigger>
                ))}
              </TabsList>
              {LOCALES.map((l) => (
                <TabsContent
                  key={l}
                  value={l}
                  className="space-y-3"
                  dir={l === "ar" ? "rtl" : "ltr"}
                >
                  <div className="grid gap-2">
                    <Label htmlFor={`headline-${l}`}>
                      {t("showcase.fields.headline")}
                      {l === orgLocale ? " *" : ""}
                    </Label>
                    <Input
                      id={`headline-${l}`}
                      value={content[l]?.headline ?? ""}
                      onChange={(e) =>
                        setContent((prev) =>
                          prev
                            ? {
                                ...prev,
                                [l]: { ...prev[l], headline: e.target.value },
                              }
                            : prev,
                        )
                      }
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor={`about-${l}`}>
                      {t("showcase.fields.about")}
                    </Label>
                    <Textarea
                      id={`about-${l}`}
                      rows={5}
                      value={content[l]?.about ?? ""}
                      onChange={(e) =>
                        setContent((prev) =>
                          prev
                            ? {
                                ...prev,
                                [l]: { ...prev[l], about: e.target.value },
                              }
                            : prev,
                        )
                      }
                    />
                  </div>
                </TabsContent>
              ))}
            </Tabs>
          </CardContent>
        </Card>

        <div className="grid gap-4 xl:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>{t("showcase.hours.title")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  setWorkingHours(
                    Object.fromEntries(
                      DAYS.map((day) => [
                        day,
                        [{ start: "09:00", end: "18:00" }],
                      ]),
                    ),
                  )
                }
              >
                {t("showcase.hours.copy")}
              </Button>
              {DAYS.map((day) => {
                const first = workingHours[day]?.[0] ?? { start: "", end: "" };
                return (
                  <div
                    key={day}
                    className="grid gap-2 sm:grid-cols-[1fr_120px_120px]"
                  >
                    <Label>{t(`showcase.days.${day}`)}</Label>
                    <Input
                      type="time"
                      value={first.start}
                      onChange={(e) =>
                        setWorkingHours((prev) => ({
                          ...prev,
                          [day]:
                            e.target.value || first.end
                              ? [{ ...first, start: e.target.value }]
                              : [],
                        }))
                      }
                    />
                    <Input
                      type="time"
                      value={first.end}
                      onChange={(e) =>
                        setWorkingHours((prev) => ({
                          ...prev,
                          [day]:
                            first.start || e.target.value
                              ? [{ ...first, end: e.target.value }]
                              : [],
                        }))
                      }
                    />
                  </div>
                );
              })}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("showcase.seo.title")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-wrap gap-2">
                {suggestions.map((chip) => (
                  <Button
                    key={chip}
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      setKeywords((prev) =>
                        prev.includes(chip)
                          ? prev
                          : [prev, chip].filter(Boolean).join(", "),
                      )
                    }
                  >
                    {chip}
                  </Button>
                ))}
              </div>
              <Label htmlFor="seo-keywords">
                {t("showcase.fields.keywords")}
              </Label>
              <Textarea
                id="seo-keywords"
                value={keywords}
                onChange={(e) => setKeywords(e.target.value)}
              />
              <div className="grid gap-3 sm:grid-cols-2">
                {SOCIALS.map((key) => (
                  <div key={key} className="grid gap-2">
                    <Label htmlFor={`social-${key}`}>
                      {t(`showcase.social.${key}`)}
                    </Label>
                    <Input
                      id={`social-${key}`}
                      value={socialLinks[key] ?? ""}
                      onChange={(e) =>
                        setSocialLinks((prev) => ({
                          ...prev,
                          [key]: e.target.value,
                        }))
                      }
                    />
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>{t("showcase.services.title")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-3 lg:grid-cols-[1fr_1.5fr_auto_auto]">
              <Input
                placeholder={t("showcase.services.title_placeholder")}
                value={serviceTitle}
                onChange={(e) => setServiceTitle(e.target.value)}
              />
              <Input
                placeholder={t("showcase.services.description_placeholder")}
                value={serviceDescription}
                onChange={(e) => setServiceDescription(e.target.value)}
              />
              <Select
                value={serviceVisible ? "visible" : "hidden"}
                onValueChange={(v) => setServiceVisible(v === "visible")}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="visible">
                    {t("showcase.services.visible")}
                  </SelectItem>
                  <SelectItem value="hidden">
                    {t("showcase.services.hidden")}
                  </SelectItem>
                </SelectContent>
              </Select>
              <Button
                type="button"
                onClick={() => addService.mutate()}
                disabled={!serviceTitle.trim() || addService.isPending}
              >
                <Plus className="size-4" />
                {t("showcase.services.add")}
              </Button>
            </div>
            <div className="space-y-2">
              {showcase.services.map((service, index) => (
                <ServiceRow
                  key={service.uuid}
                  service={service}
                  locale={activeLocale}
                  onMove={(direction) =>
                    reorderServices.mutate(
                      move(showcase.services, index, direction).map(
                        (s) => s.uuid,
                      ),
                    )
                  }
                  onDelete={() => removeService.mutate(service.uuid)}
                />
              ))}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("showcase.gallery.title")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-3 md:grid-cols-[1fr_220px]">
              <Input
                value={photoCaption}
                onChange={(e) => setPhotoCaption(e.target.value)}
                placeholder={t("showcase.gallery.caption")}
              />
              <label>
                <span
                  className={
                    photoUploadDisabled || uploadPhoto.isPending
                      ? "pointer-events-none opacity-50"
                      : undefined
                  }
                >
                  <Button asChild>
                    <span>
                      {uploadPhoto.isPending ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <ImagePlus className="size-4" />
                      )}
                      {t("showcase.gallery.upload")}
                    </span>
                  </Button>
                </span>
                <input
                  className="sr-only"
                  type="file"
                  accept="image/jpeg,image/png,image/webp"
                  disabled={photoUploadDisabled || uploadPhoto.isPending}
                  onChange={(e) => {
                    const file = e.target.files?.[0];
                    e.currentTarget.value = "";
                    if (!file) return;
                    if (showcase.photos.length >= photoLimit) {
                      setPhotoError(t("showcase.gallery.limit_reached"));
                      return;
                    }
                    if (file.size > MAX_PHOTO_BYTES) {
                      setPhotoError(t("showcase.gallery.too_large"));
                      return;
                    }
                    uploadPhoto.mutate(file);
                  }}
                />
              </label>
            </div>
            {photoUploadDisabled ? (
              <p
                className="text-muted-foreground text-sm"
                data-testid="photo-limit"
              >
                {t("showcase.gallery.limit_reached")}
              </p>
            ) : null}
            {photoError ? (
              <p className="text-destructive text-sm" role="alert">
                {photoError}
              </p>
            ) : null}
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              {showcase.photos.map((photo, index) => (
                <div
                  key={photo.uuid}
                  className="border-border rounded-md border p-2"
                >
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={showcaseService.photoUrl(photo.uuid, target)}
                    alt={photo.caption[activeLocale] ?? ""}
                    className="bg-muted aspect-[4/3] w-full rounded object-cover"
                  />
                  <p className="text-muted-foreground mt-2 truncate text-xs">
                    {photo.caption[activeLocale] ?? photo.caption.tr}
                  </p>
                  <div className="mt-2 flex gap-1">
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      onClick={() =>
                        reorderPhotos.mutate(
                          move(showcase.photos, index, -1).map((p) => p.uuid),
                        )
                      }
                    >
                      <ArrowUp className="size-4" />
                    </Button>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      onClick={() =>
                        reorderPhotos.mutate(
                          move(showcase.photos, index, 1).map((p) => p.uuid),
                        )
                      }
                    >
                      <ArrowDown className="size-4" />
                    </Button>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      onClick={() => removePhoto.mutate(photo.uuid)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("showcase.rating.title")}</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4 lg:grid-cols-[1fr_120px_160px_auto]">
            <Input
              value={placeId}
              onChange={(e) => setPlaceId(e.target.value)}
              placeholder={t("showcase.rating.place_id")}
              disabled={showcase.places_configured}
            />
            <Input
              value={rating}
              onChange={(e) => setRating(e.target.value)}
              inputMode="decimal"
              placeholder="4.8"
              disabled={!showcase.manual_rating_allowed}
            />
            <Input
              value={reviewCount}
              onChange={(e) => setReviewCount(e.target.value)}
              inputMode="numeric"
              placeholder="128"
              disabled={!showcase.manual_rating_allowed}
            />
            <Button
              type="button"
              variant="outline"
              onClick={() => saveRating.mutate()}
              disabled={!showcase.manual_rating_allowed || saveRating.isPending}
            >
              {t("showcase.rating.save")}
            </Button>
            <p className="text-muted-foreground text-sm lg:col-span-4">
              {showcase.places_configured
                ? t("showcase.rating.places_readonly")
                : t("showcase.rating.manual")}
            </p>
          </CardContent>
        </Card>
      </div>
    </EntityPage>
  );
}

function ServiceRow({
  service,
  locale,
  onMove,
  onDelete,
}: {
  service: ShowcaseService;
  locale: string;
  onMove: (direction: -1 | 1) => void;
  onDelete: () => void;
}) {
  const { t } = useLocale();
  return (
    <div className="border-border grid gap-2 rounded-md border p-3 md:grid-cols-[1fr_auto]">
      <div>
        <p className="font-medium">
          {service.title[locale] ||
            service.title.tr ||
            service.category?.name ||
            t("showcase.services.custom")}
        </p>
        <p className="text-muted-foreground text-sm">
          {service.description[locale] ?? service.description.tr}
        </p>
      </div>
      <div className="flex gap-1">
        <Button
          type="button"
          size="icon"
          variant="ghost"
          onClick={() => onMove(-1)}
        >
          <ArrowUp className="size-4" />
        </Button>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          onClick={() => onMove(1)}
        >
          <ArrowDown className="size-4" />
        </Button>
        <Button type="button" size="icon" variant="ghost" onClick={onDelete}>
          <Trash2 className="size-4" />
        </Button>
      </div>
    </div>
  );
}

export function ShowcaseEditorPage({ orgUuid }: { orgUuid?: string }) {
  const params = useParams<{ slug: string }>();
  const slug = String(params.slug ?? "");
  const activeOrg = useActiveOrganization(slug);
  const { t } = useLocale();
  const [selectedOrg, setSelectedOrg] = useState(
    orgUuid ?? activeOrg?.uuid ?? "",
  );
  const targetOrg = orgUuid ?? (selectedOrg || activeOrg?.uuid);
  return (
    <FeatureGuard slug={slug} feature="dealer_showcase">
      {activeOrg?.type === "distributor" && !orgUuid ? (
        <Card className="mb-4">
          <CardContent className="grid gap-2 pt-6 md:max-w-md">
            <Label htmlFor="showcase-target-org">
              {t("showcase.dealer_selector.label")}
            </Label>
            <Input
              id="showcase-target-org"
              value={selectedOrg}
              onChange={(event) => setSelectedOrg(event.target.value)}
              placeholder={t("showcase.dealer_selector.placeholder")}
            />
          </CardContent>
        </Card>
      ) : null}
      <EditorInner key={targetOrg} slug={slug} orgUuid={targetOrg} />
    </FeatureGuard>
  );
}
