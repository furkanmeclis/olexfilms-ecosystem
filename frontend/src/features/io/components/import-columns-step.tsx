"use client";

import { ChevronDown, Sparkles } from "lucide-react";
import { useMemo } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { MAPPING_SKIP } from "@/features/io/lib/suggest-mapping";
import type { ImportSchemaField } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

type ImportColumnsStepProps = {
  resource: string;
  schema: ImportSchemaField[];
  headers: string[];
  samples: Record<string, string>[];
  /** Field key → file header. */
  mapping: Record<string, string>;
  /** Fields whose column was matched automatically (and left as is). */
  autoFields: Set<string>;
  defaults: Record<string, string>;
  /** Job defaults the caller fixes (not editable here). */
  fixedDefaults?: Record<string, string>;
  error: string | null;
  onMappingChange: (fieldKey: string, header: string) => void;
  onDefaultChange: (fieldKey: string, value: string) => void;
  onAutoMatch: () => void;
};

export function isFieldMapped(mapping: Record<string, string>, key: string) {
  return Boolean(mapping[key] && mapping[key] !== MAPPING_SKIP);
}

/** Required fields with neither a file column nor a default value. */
export function missingRequiredFields(
  schema: ImportSchemaField[],
  mapping: Record<string, string>,
  defaults: Record<string, string>,
  fixedDefaults?: Record<string, string>,
) {
  return schema.filter(
    (f) =>
      f.required &&
      !isFieldMapped(mapping, f.key) &&
      !defaults[f.key]?.trim() &&
      !fixedDefaults?.[f.key],
  );
}

function sampleValues(samples: Record<string, string>[], header?: string) {
  if (!header || header === MAPPING_SKIP) return [];
  return samples
    .map((row) => row[header]?.trim() ?? "")
    .filter(Boolean)
    .slice(0, 3);
}

/**
 * Step 2: a file column per schema field (auto-match badge, sample values
 * of the first rows, required marks) and default values for the fields the
 * file does not have.
 */
export function ImportColumnsStep({
  resource,
  schema,
  headers,
  samples,
  mapping,
  autoFields,
  defaults,
  fixedDefaults,
  error,
  onMappingChange,
  onDefaultChange,
  onAutoMatch,
}: ImportColumnsStepProps) {
  const { t } = useLocale();

  const usedHeaders = useMemo(() => {
    const set = new Set<string>();
    for (const header of Object.values(mapping)) {
      if (header && header !== MAPPING_SKIP) set.add(header);
    }
    return set;
  }, [mapping]);

  const unmapped = schema.filter(
    (f) => !isFieldMapped(mapping, f.key) && !fixedDefaults?.[f.key],
  );
  const requiredUnmapped = unmapped.filter((f) => f.required);
  const optionalUnmapped = unmapped.filter((f) => !f.required);
  const mappedCount = schema.filter((f) =>
    isFieldMapped(mapping, f.key),
  ).length;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium">{t("imports.mapping_title")}</p>
          <p className="text-muted-foreground text-sm">
            {t("imports.columns_hint", {
              mapped: mappedCount,
              total: schema.length,
              columns: headers.length,
            })}
          </p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={onAutoMatch}>
          <Sparkles className="size-3.5" />
          {t("imports.mapping_auto")}
        </Button>
      </div>

      {error ? (
        <p className="text-destructive text-sm" role="alert">
          {error}
        </p>
      ) : null}

      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm" data-testid="import-columns">
          <thead className="bg-muted/40 text-muted-foreground text-xs">
            <tr>
              <th className="px-3 py-2 text-start font-medium">
                {t("imports.columns.field")}
              </th>
              <th className="px-3 py-2 text-start font-medium">
                {t("imports.columns.file_column")}
              </th>
              <th className="px-3 py-2 text-start font-medium">
                {t("imports.columns.samples")}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {schema.map((field) => {
              const header = mapping[field.key];
              const values = sampleValues(samples, header);
              const fixed = fixedDefaults?.[field.key];
              return (
                <tr key={field.key} data-field={field.key}>
                  <td className="px-3 py-2 align-middle">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="font-medium">{t(field.labelKey)}</span>
                      {field.required ? (
                        <Badge variant="warning">{t("imports.required")}</Badge>
                      ) : null}
                    </div>
                  </td>
                  <td className="px-3 py-2 align-middle">
                    {fixed ? (
                      <span className="text-muted-foreground text-xs">
                        {t("imports.columns.fixed")}
                      </span>
                    ) : (
                      <div className="flex items-center gap-2">
                        <Select
                          value={header || MAPPING_SKIP}
                          onValueChange={(v) => onMappingChange(field.key, v)}
                        >
                          <SelectTrigger
                            className="min-w-44"
                            aria-label={t(field.labelKey)}
                          >
                            <SelectValue
                              placeholder={t("imports.mapping_skip")}
                            />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value={MAPPING_SKIP}>
                              {t("imports.mapping_skip")}
                            </SelectItem>
                            {headers.map((h) => (
                              <SelectItem
                                key={h}
                                value={h}
                                disabled={usedHeaders.has(h) && header !== h}
                              >
                                {h}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        {autoFields.has(field.key) &&
                        isFieldMapped(mapping, field.key) ? (
                          <Badge variant="success" className="gap-1">
                            <Sparkles className="size-3" />
                            {t("imports.columns.auto")}
                          </Badge>
                        ) : null}
                      </div>
                    )}
                  </td>
                  <td className="text-muted-foreground px-3 py-2 align-middle text-xs">
                    {values.length ? (
                      <span className="line-clamp-2 break-all">
                        {values.join(" · ")}
                      </span>
                    ) : (
                      "—"
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {requiredUnmapped.length ? (
        <section
          className="space-y-3 rounded-lg border border-amber-500/40 bg-amber-500/5 p-4"
          data-testid="import-required-defaults"
        >
          <div className="space-y-1">
            <p className="text-sm font-medium">
              {t("imports.defaults_required_title")}
            </p>
            <p className="text-muted-foreground text-sm">
              {t("imports.defaults_required_hint")}
            </p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            {requiredUnmapped.map((field) => (
              <div key={field.key} className="grid gap-1.5">
                <Label htmlFor={`import-default-${field.key}`}>
                  {t(field.labelKey)} *
                </Label>
                <Input
                  id={`import-default-${field.key}`}
                  value={defaults[field.key] ?? ""}
                  onChange={(e) => onDefaultChange(field.key, e.target.value)}
                />
              </div>
            ))}
          </div>
        </section>
      ) : null}

      {optionalUnmapped.length ? (
        <Collapsible
          defaultOpen={optionalUnmapped.some((f) => defaults[f.key])}
        >
          <CollapsibleTrigger asChild>
            <Button type="button" variant="ghost" size="sm" className="px-0">
              <ChevronDown className="size-4" />
              {t("imports.defaults_optional_title", {
                count: optionalUnmapped.length,
              })}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="space-y-3 pt-2">
            <p className="text-muted-foreground text-sm">
              {t("imports.defaults_hint")}
            </p>
            {resource === "platform.users" ? (
              <p className="text-muted-foreground text-xs">
                {t("imports.defaults_users_status")} ·{" "}
                {t("imports.defaults_users_locale")}
              </p>
            ) : null}
            <div className="grid gap-3 sm:grid-cols-2">
              {optionalUnmapped.map((field) => (
                <div key={field.key} className="grid gap-1.5">
                  <Label htmlFor={`import-default-${field.key}`}>
                    {t(field.labelKey)}
                  </Label>
                  <Input
                    id={`import-default-${field.key}`}
                    value={defaults[field.key] ?? ""}
                    placeholder={t("imports.defaults_optional")}
                    onChange={(e) => onDefaultChange(field.key, e.target.value)}
                  />
                </div>
              ))}
            </div>
          </CollapsibleContent>
        </Collapsible>
      ) : null}
    </div>
  );
}
