"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";

import { EditorX } from "@/components/editor/editor-x";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { statusVariant } from "@/features/document-templates/components/document-templates-page";
import {
  LexicalVariableMenu,
  VariableMenu,
} from "@/features/document-templates/components/variable-menu";
import {
  useDocumentKinds,
  useDocumentTemplate,
  useDocumentTemplates,
  useDocumentTemplateVersions,
  usePublishDocumentTemplate,
  useSaveDocumentTemplateDraft,
} from "@/features/document-templates/hooks/use-document-templates";
import {
  insertAt,
  placeholder,
  unknownPlaceholders,
} from "@/features/document-templates/lib/variables";
import {
  documentLanguage,
  documentTemplatesService,
  type DocumentKind,
  type DocumentTemplate,
} from "@/features/document-templates/services/document-templates.service";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type EditorMode = "html" | "visual";

type DocumentTemplateEditorProps =
  | { uuid: string; kind?: never; language?: never; brandSlug?: never }
  | { uuid?: never; kind: DocumentKind; language: string; brandSlug?: string };

type Draft = {
  kind: DocumentKind;
  language: string;
  brandSlug: string;
  name: string;
  html: string;
  stateJson: string | null;
  source?: DocumentTemplate;
};

/** Template editor: HTML source or Lexical (EditorX), variable menu, PDF preview. */
export function DocumentTemplateEditor(props: DocumentTemplateEditorProps) {
  const { t } = useLocale();
  const existing = useDocumentTemplate(props.uuid ?? null);
  const baseList = useDocumentTemplates(
    { kind: props.kind, current: true },
    Boolean(props.kind),
  );

  // a new language starts from the active template of the requested
  // language, else en, fetched in full (lists carry no HTML)
  const baseRow = useMemo(() => {
    const rows = (baseList.data?.items ?? []).filter(
      (r) => r.status === "active",
    );
    return (
      rows.find((r) => documentLanguage(r.language) === props.language) ??
      rows.find((r) => r.language === "en") ??
      rows[0]
    );
  }, [baseList.data, props.language]);
  const baseFull = useDocumentTemplate(
    props.uuid ? null : (baseRow?.uuid ?? null),
  );

  const initial = useMemo<Draft | null>(() => {
    if (props.uuid) {
      const row = existing.data;
      if (!row) return null;
      return {
        kind: row.kind,
        language: row.language,
        brandSlug: row.brand_slug ?? "",
        name: row.name,
        html: row.html ?? "",
        stateJson: row.lexical_json ? JSON.stringify(row.lexical_json) : null,
        source: row,
      };
    }
    if (!props.kind || !baseList.data) return null;
    if (baseRow && !baseFull.data) return null;
    return {
      kind: props.kind,
      language: props.language,
      brandSlug: props.brandSlug ?? "",
      name: baseRow?.name ?? props.kind,
      html: baseFull.data?.html ?? "",
      stateJson: null,
    };
  }, [
    props.uuid,
    props.kind,
    props.language,
    props.brandSlug,
    existing.data,
    baseList.data,
    baseRow,
    baseFull.data,
  ]);

  const loading = props.uuid ? existing.isLoading : baseList.isLoading;
  const failed = props.uuid ? existing.isError : baseList.isError;

  return (
    <EntityPage
      title={t("documents.editor.title")}
      description={t("documents.editor.description")}
      permission={permissions.documentTemplates.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("documents.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("documents.title"),
          href: routes.platform.documentTemplates.root,
        },
        { label: t("documents.editor.title") },
      ]}
    >
      {loading ? <Loading label={t("common.loading")} /> : null}
      {failed ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("documents.load_failed")}
        />
      ) : null}
      {initial ? (
        <EditorBody key={initial.source?.uuid ?? "new"} initial={initial} />
      ) : null}
    </EntityPage>
  );
}

function EditorBody({ initial }: { initial: Draft }) {
  const { t } = useLocale();
  const router = useRouter();
  const { can } = usePermission();
  const canWrite = can(permissions.documentTemplates.write);
  const kinds = useDocumentKinds();
  const save = useSaveDocumentTemplateDraft();
  const publish = usePublishDocumentTemplate();

  const [name, setName] = useState(initial.name);
  const [html, setHtml] = useState(initial.html);
  const [stateJson, setStateJson] = useState<string | null>(initial.stateJson);
  const [mode, setMode] = useState<EditorMode>(
    initial.stateJson ? "visual" : "html",
  );
  const [editorKey, setEditorKey] = useState(0);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const current = initial.source;
  const versions = useDocumentTemplateVersions(current?.uuid ?? null);

  useEffect(
    () => () => {
      if (previewUrl) URL.revokeObjectURL(previewUrl);
    },
    [previewUrl],
  );

  const variables = useMemo(
    () => kinds.data?.find((k) => k.kind === initial.kind)?.variables ?? [],
    [kinds.data, initial.kind],
  );
  const unknown = unknownPlaceholders(html, variables);
  const dirty =
    html !== (current?.html ?? "") || name !== (current?.name ?? "");

  const insertIntoTextarea = (key: string) => {
    const el = textareaRef.current;
    const next = insertAt(
      html,
      el?.selectionStart ?? html.length,
      el?.selectionEnd ?? html.length,
      placeholder(key),
    );
    setHtml(next.value);
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(next.caret, next.caret);
    });
  };

  const switchMode = (next: EditorMode) => {
    if (next === mode) return;
    if (next === "visual") {
      setStateJson(null);
      setEditorKey((k) => k + 1);
    }
    setMode(next);
  };

  const saveDraft = async () => {
    const row = await save.mutateAsync({
      kind: initial.kind,
      language: initial.language,
      brand_slug: initial.brandSlug || undefined,
      name,
      html,
      lexical_json:
        mode === "visual" && stateJson ? JSON.parse(stateJson) : null,
    });
    if (row.uuid !== current?.uuid) {
      router.replace(routes.platform.documentTemplates.edit(row.uuid));
    }
    return row;
  };

  const saveAndPublish = async () => {
    const draft =
      current?.status === "draft" && !dirty ? current : await saveDraft();
    const row = await publish.mutateAsync(draft.uuid);
    if (row.uuid !== current?.uuid) {
      router.replace(routes.platform.documentTemplates.edit(row.uuid));
    }
  };

  const runPreview = async () => {
    setPreviewing(true);
    try {
      const blob = await documentTemplatesService.preview({
        kind: initial.kind,
        html,
        locale: initial.language,
      });
      setPreviewUrl(URL.createObjectURL(blob));
    } catch (err) {
      appToast.error(
        err instanceof Error
          ? err.message
          : t("documents.toast.preview_failed"),
      );
    } finally {
      setPreviewing(false);
    }
  };

  const rtl = initial.language === "ar";

  return (
    <div className="grid gap-6 xl:grid-cols-2">
      <div className="grid min-w-0 content-start gap-4">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <Badge variant="secondary">
            {t(`documents.kinds.${initial.kind}`)}
          </Badge>
          <Badge variant="outline">
            {t(`documents.languages.${documentLanguage(initial.language)}`)}
          </Badge>
          <Badge variant="outline">
            {initial.brandSlug || t("documents.brand_default")}
          </Badge>
          {current ? (
            <Badge variant={statusVariant(current.status)}>
              v{current.version} · {t(`documents.status.${current.status}`)}
            </Badge>
          ) : (
            <Badge variant="warning">{t("documents.status.new")}</Badge>
          )}
        </div>

        <div className="grid gap-1.5">
          <Label htmlFor="template-name">{t("documents.fields.name")}</Label>
          <Input
            id="template-name"
            value={name}
            maxLength={150}
            disabled={!canWrite}
            onChange={(e) => setName(e.target.value)}
          />
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2">
          <Tabs value={mode} onValueChange={(v) => switchMode(v as EditorMode)}>
            <TabsList>
              <TabsTrigger value="html">
                {t("documents.editor.mode_html")}
              </TabsTrigger>
              <TabsTrigger value="visual">
                {t("documents.editor.mode_visual")}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          {mode === "html" ? (
            <VariableMenu
              variables={variables}
              onInsert={insertIntoTextarea}
              disabled={!canWrite}
            />
          ) : null}
        </div>
        {mode === "visual" ? (
          <p className="text-muted-foreground text-xs">
            {t("documents.editor.visual_hint")}
          </p>
        ) : null}

        {mode === "html" ? (
          <Textarea
            ref={textareaRef}
            dir={rtl ? "rtl" : "ltr"}
            className="min-h-[28rem] font-mono text-xs"
            value={html}
            disabled={!canWrite}
            onChange={(e) => setHtml(e.target.value)}
            spellCheck={false}
          />
        ) : (
          <div dir={rtl ? "rtl" : "ltr"}>
            <EditorX
              key={editorKey}
              initialStateJson={stateJson}
              initialHtml={html}
              editable={canWrite}
              onHtmlChange={(nextHtml, json) => {
                setHtml(nextHtml);
                setStateJson(json);
              }}
              toolbarExtra={<LexicalVariableMenu variables={variables} />}
            />
          </div>
        )}

        {unknown.length > 0 ? (
          <p className="text-destructive text-sm">
            {t("documents.editor.unknown_variables")}: {unknown.join(", ")}
          </p>
        ) : null}

        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            onClick={() => void runPreview()}
            disabled={previewing || !html.trim()}
          >
            {previewing
              ? t("documents.actions.previewing")
              : t("documents.actions.preview")}
          </Button>
          {canWrite ? (
            <>
              <Button
                variant="secondary"
                onClick={() => void saveDraft()}
                disabled={save.isPending || unknown.length > 0 || !html.trim()}
              >
                {t("documents.actions.save_draft")}
              </Button>
              <Button
                onClick={() => void saveAndPublish()}
                disabled={
                  save.isPending ||
                  publish.isPending ||
                  unknown.length > 0 ||
                  !html.trim() ||
                  (current?.status === "active" && !dirty)
                }
              >
                {t("documents.actions.publish")}
              </Button>
            </>
          ) : null}
        </div>
        <p className="text-muted-foreground text-xs">
          {t("documents.editor.publish_hint")}
        </p>

        {versions.data && versions.data.length > 0 ? (
          <div className="rounded-lg border">
            <h3 className="border-b px-4 py-2 text-sm font-semibold">
              {t("documents.editor.versions")}
            </h3>
            <ul className="divide-y text-sm">
              {versions.data.map((v) => (
                <li
                  key={v.uuid}
                  className="flex items-center justify-between gap-2 px-4 py-2"
                >
                  <span>
                    v{v.version} · {v.name}
                  </span>
                  <span className="flex items-center gap-2">
                    <Badge variant={statusVariant(v.status)}>
                      {t(`documents.status.${v.status}`)}
                    </Badge>
                    {v.uuid !== current?.uuid ? (
                      <Link
                        className="text-primary text-xs underline-offset-4 hover:underline"
                        href={routes.platform.documentTemplates.edit(v.uuid)}
                      >
                        {t("documents.actions.open")}
                      </Link>
                    ) : null}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </div>

      <div className="grid min-w-0 content-start gap-2">
        <h3 className="text-sm font-semibold">
          {t("documents.editor.preview")}
        </h3>
        {previewUrl ? (
          <iframe
            title={t("documents.editor.preview")}
            src={previewUrl}
            className="h-[48rem] w-full rounded-lg border"
          />
        ) : (
          <div className="text-muted-foreground flex h-64 items-center justify-center rounded-lg border border-dashed text-sm">
            {t("documents.editor.preview_empty")}
          </div>
        )}
      </div>
    </div>
  );
}
