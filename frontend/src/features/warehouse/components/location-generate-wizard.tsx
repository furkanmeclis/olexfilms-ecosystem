"use client";

import { useMutation } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Plus, Trash2 } from "lucide-react";
import { useId, useMemo, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
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
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  defaultLevels,
  fixLevelChain,
  GENERATE_MAX,
  levelCodes,
  newLevel,
  planGeneration,
  toGenerateLevel,
  type LevelDraft,
  type LevelError,
} from "@/features/warehouse/lib/generate";
import {
  allowedChildTypes,
  type LocationNode,
} from "@/features/warehouse/lib/tree";
import {
  warehouseService,
  type WarehouseGenerateResult,
  type WarehouseLocationType,
} from "@/features/warehouse/services/warehouse.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type Step = "structure" | "preview" | "result";
const STEPS: Step[] = ["structure", "preview", "result"];

/**
 * Bulk location generation (TEC-764): structure (one level per location
 * type, a number range or a code list) → preview (resulting codes, totals,
 * codes that already exist, type conflicts; an extra confirmation above
 * GENERATE_CONFIRM_THRESHOLD) → one POST /v1/warehouse/locations/generate
 * → result. `parent` null = the room root.
 */
export function LocationGenerateWizard({
  open,
  onOpenChange,
  roomUuid,
  baseCode,
  parent,
  roots,
  onDone,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  roomUuid: string;
  /** full_code of the parent (WAREHOUSE-ROOM at the root). */
  baseCode: string;
  parent: LocationNode | null;
  /** The room's current tree (to spot existing codes). */
  roots: LocationNode[];
  onDone: (result: WarehouseGenerateResult) => void;
}) {
  const { t } = useLocale();
  const parentType = parent?.type ?? null;
  const [step, setStep] = useState<Step>("structure");
  const [levels, setLevels] = useState<LevelDraft[]>(() =>
    defaultLevels(parentType),
  );
  const [largeOk, setLargeOk] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const plan = useMemo(
    () => planGeneration({ levels, parent, baseCode, roots }),
    [levels, parent, baseCode, roots],
  );

  const generate = useMutation({
    mutationFn: () =>
      warehouseService.generateLocations({
        room_uuid: roomUuid,
        parent_uuid: parent?.uuid ?? null,
        levels: levels.map(toGenerateLevel),
      }),
    onSuccess: (result) => {
      setStep("result");
      onDone(result);
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.bulk.failed"))),
  });

  const setLevel = (i: number, patch: Partial<LevelDraft>) => {
    setLargeOk(false);
    setLevels((prev) =>
      fixLevelChain(
        parentType,
        prev.map((lv, j) => (j === i ? { ...lv, ...patch } : lv)),
      ),
    );
  };
  const lastType = levels.at(-1)?.type ?? parentType;
  const nextType = allowedChildTypes(lastType)[0];
  const structureOk =
    levels.length > 0 && plan.levelErrors.every((e) => e === null);
  const canCreate =
    plan.ok &&
    plan.created > 0 &&
    (!plan.needsExtraConfirm || largeOk) &&
    !generate.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
        data-testid="bulk-wizard"
        data-step={step}
      >
        <DialogHeader>
          <DialogTitle>{t("warehouse.bulk.title")}</DialogTitle>
          <DialogDescription>
            {t("warehouse.bulk.description")}
          </DialogDescription>
          <p className="text-muted-foreground font-mono text-xs" dir="ltr">
            {baseCode}
          </p>
        </DialogHeader>

        <ol
          className="flex flex-wrap gap-2 text-sm"
          aria-label={t("warehouse.bulk.steps")}
        >
          {STEPS.map((s, i) => (
            <li
              key={s}
              aria-current={s === step ? "step" : undefined}
              className={cn(
                "rounded-full border px-3 py-0.5",
                s === step
                  ? "border-primary bg-primary text-primary-foreground"
                  : "text-muted-foreground",
              )}
            >
              {i + 1}. {t(`warehouse.bulk.step.${s}`)}
            </li>
          ))}
        </ol>

        {step === "structure" ? (
          <div className="space-y-4">
            {levels.map((lv, i) => (
              <LevelEditor
                key={i}
                index={i}
                level={lv}
                types={allowedChildTypes(
                  i === 0 ? parentType : levels[i - 1].type,
                )}
                error={plan.levelErrors[i] ?? null}
                removable={i === levels.length - 1 && levels.length > 1}
                onChange={(patch) => setLevel(i, patch)}
                onRemove={() => setLevels((prev) => prev.slice(0, -1))}
              />
            ))}
            {nextType ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() =>
                  setLevels((prev) => [...prev, newLevel(nextType)])
                }
                data-testid="bulk-add-level"
              >
                <Plus className="size-4" />
                {t("warehouse.bulk.add_level", {
                  type: t(`warehouse.type.${nextType}`),
                })}
              </Button>
            ) : null}
          </div>
        ) : null}

        {step === "preview" ? (
          <div className="space-y-4" data-testid="bulk-preview">
            <dl className="grid gap-3 sm:grid-cols-3">
              <Stat
                label={t("warehouse.bulk.preview.new")}
                value={plan.created}
                testId="bulk-preview-new"
              />
              <Stat
                label={t("warehouse.bulk.preview.existing")}
                value={plan.existing}
                testId="bulk-preview-existing"
              />
              <Stat
                label={t("warehouse.bulk.preview.total")}
                value={plan.total}
                testId="bulk-preview-total"
              />
            </dl>
            <p className="text-sm" data-testid="bulk-preview-levels">
              {levels
                .map(
                  (lv, i) =>
                    `${plan.levelCounts[i]} ${t(`warehouse.type.${lv.type}`)}`,
                )
                .join(" × ")}
            </p>
            {plan.sample.length > 0 ? (
              <div className="space-y-1">
                <p className="text-sm font-medium">
                  {t("warehouse.bulk.preview.sample")}
                </p>
                <ul
                  className="bg-muted/40 space-y-0.5 rounded-md p-3 font-mono text-xs"
                  dir="ltr"
                  data-testid="bulk-preview-sample"
                >
                  {plan.sample.map((code, i) => (
                    <li key={code}>
                      {i === plan.sample.length - 1 &&
                      plan.leaves > plan.sample.length ? (
                        <span className="text-muted-foreground">… </span>
                      ) : null}
                      {code}
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            {plan.tooMany ? (
              <Alert variant="destructive" data-testid="bulk-too-many">
                <AlertTriangle className="size-4" />
                <AlertDescription>
                  {t("warehouse.bulk.preview.too_many", {
                    count: plan.total,
                    max: GENERATE_MAX,
                  })}
                </AlertDescription>
              </Alert>
            ) : null}
            {plan.conflicts.length > 0 ? (
              <Alert variant="destructive" data-testid="bulk-conflicts">
                <AlertTriangle className="size-4" />
                <AlertTitle>{t("warehouse.bulk.preview.conflicts")}</AlertTitle>
                <AlertDescription>
                  <ul className="font-mono text-xs" dir="ltr">
                    {plan.conflicts.slice(0, 10).map((c) => (
                      <li key={c.full_code}>
                        {t("warehouse.bulk.preview.conflict_row", {
                          code: c.full_code,
                          existing: t(`warehouse.type.${c.existing}`),
                          wanted: t(`warehouse.type.${c.wanted}`),
                        })}
                      </li>
                    ))}
                  </ul>
                </AlertDescription>
              </Alert>
            ) : null}
            {plan.ok && plan.created === 0 ? (
              <Alert data-testid="bulk-nothing">
                <AlertDescription>
                  {t("warehouse.bulk.preview.nothing")}
                </AlertDescription>
              </Alert>
            ) : null}
            {plan.ok && plan.existing > 0 && plan.created > 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("warehouse.bulk.preview.existing_hint")}
              </p>
            ) : null}
            {plan.ok && plan.needsExtraConfirm ? (
              <Alert data-testid="bulk-large">
                <AlertTriangle className="size-4" />
                <AlertTitle>
                  {t("warehouse.bulk.preview.large", { count: plan.created })}
                </AlertTitle>
                <AlertDescription>
                  <label className="mt-2 flex items-center gap-2">
                    <Checkbox
                      checked={largeOk}
                      onCheckedChange={(v) => setLargeOk(v === true)}
                      data-testid="bulk-large-confirm"
                    />
                    {t("warehouse.bulk.preview.large_confirm", {
                      count: plan.created,
                    })}
                  </label>
                </AlertDescription>
              </Alert>
            ) : null}
            {error ? (
              <p
                role="alert"
                className="text-destructive text-sm"
                data-testid="bulk-error"
              >
                {error}
              </p>
            ) : null}
          </div>
        ) : null}

        {step === "result" && generate.data ? (
          <div className="space-y-2" data-testid="bulk-result">
            <p className="flex items-center gap-2 font-medium">
              <CheckCircle2 className="size-5 text-emerald-600" />
              {t("warehouse.bulk.result.created", {
                count: generate.data.created,
              })}
            </p>
            {generate.data.existing > 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("warehouse.bulk.result.existing", {
                  count: generate.data.existing,
                })}
              </p>
            ) : null}
          </div>
        ) : null}

        <DialogFooter>
          {step === "structure" ? (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {t("warehouse.form.cancel")}
              </Button>
              <Button
                type="button"
                disabled={!structureOk}
                onClick={() => {
                  setError(null);
                  setStep("preview");
                }}
                data-testid="bulk-next"
              >
                {t("warehouse.bulk.next")}
              </Button>
            </>
          ) : null}
          {step === "preview" ? (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => setStep("structure")}
                data-testid="bulk-back"
              >
                {t("warehouse.bulk.back")}
              </Button>
              <Button
                type="button"
                disabled={!canCreate}
                aria-busy={generate.isPending}
                onClick={() => {
                  setError(null);
                  generate.mutate();
                }}
                data-testid="bulk-create"
              >
                {t("warehouse.bulk.create", { count: plan.created })}
              </Button>
            </>
          ) : null}
          {step === "result" ? (
            <Button
              type="button"
              onClick={() => onOpenChange(false)}
              data-testid="bulk-close"
            >
              {t("warehouse.bulk.close")}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Stat({
  label,
  value,
  testId,
}: {
  label: string;
  value: number;
  testId: string;
}) {
  const { format } = useLocale();
  return (
    <div className="rounded-md border p-3">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-2xl font-semibold tabular-nums" data-testid={testId}>
        {format.number(value)}
      </dd>
    </div>
  );
}

function LevelEditor({
  index,
  level,
  types,
  error,
  removable,
  onChange,
  onRemove,
}: {
  index: number;
  level: LevelDraft;
  types: WarehouseLocationType[];
  error: LevelError | null;
  removable: boolean;
  onChange: (patch: Partial<LevelDraft>) => void;
  onRemove: () => void;
}) {
  const { t } = useLocale();
  const id = useId();
  const result = levelCodes(level);
  const example = result.ok
    ? result.codes.length > 3
      ? `${result.codes.slice(0, 2).join(", ")} … ${result.codes.at(-1)}`
      : result.codes.join(", ")
    : "";

  return (
    <fieldset
      className="space-y-3 rounded-md border p-3"
      data-testid="bulk-level"
      data-type={level.type}
    >
      <legend className="px-1 text-sm font-medium">
        {t("warehouse.bulk.level_title", { n: index + 1 })}
      </legend>
      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <Label htmlFor={`${id}-type`}>{t("warehouse.fields.type")}</Label>
          <Select
            value={level.type}
            onValueChange={(v) =>
              onChange({ type: v as WarehouseLocationType })
            }
          >
            <SelectTrigger
              id={`${id}-type`}
              className="w-36"
              data-testid="bulk-level-type"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {types.map((ty) => (
                <SelectItem key={ty} value={ty}>
                  {t(`warehouse.type.${ty}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div
          className="flex gap-1"
          role="group"
          aria-label={t("warehouse.bulk.mode_label")}
        >
          {(["range", "list"] as const).map((m) => (
            <Button
              key={m}
              type="button"
              size="sm"
              variant={level.mode === m ? "default" : "outline"}
              aria-pressed={level.mode === m}
              onClick={() => onChange({ mode: m })}
              data-testid={`bulk-level-mode-${m}`}
            >
              {t(`warehouse.bulk.mode.${m}`)}
            </Button>
          ))}
        </div>
        {removable ? (
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            className="ms-auto"
            aria-label={t("warehouse.bulk.remove_level")}
            title={t("warehouse.bulk.remove_level")}
            onClick={onRemove}
          >
            <Trash2 className="size-4" />
          </Button>
        ) : null}
      </div>

      {level.mode === "range" ? (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Field
            id={`${id}-prefix`}
            label={t("warehouse.bulk.prefix")}
            hint={t("warehouse.bulk.prefix_hint")}
            value={level.prefix}
            onChange={(v) => onChange({ prefix: v })}
            testId="bulk-level-prefix"
          />
          <Field
            id={`${id}-from`}
            label={t("warehouse.bulk.from")}
            value={level.from}
            inputMode="numeric"
            onChange={(v) => onChange({ from: v })}
            testId="bulk-level-from"
          />
          <Field
            id={`${id}-to`}
            label={t("warehouse.bulk.to")}
            value={level.to}
            inputMode="numeric"
            onChange={(v) => onChange({ to: v })}
            testId="bulk-level-to"
          />
          <Field
            id={`${id}-pad`}
            label={t("warehouse.bulk.pad")}
            hint={t("warehouse.bulk.pad_hint")}
            value={level.pad}
            inputMode="numeric"
            onChange={(v) => onChange({ pad: v })}
            testId="bulk-level-pad"
          />
        </div>
      ) : (
        <Field
          id={`${id}-codes`}
          label={t("warehouse.bulk.codes")}
          hint={t("warehouse.bulk.codes_hint")}
          value={level.codes}
          onChange={(v) => onChange({ codes: v })}
          testId="bulk-level-codes"
        />
      )}

      {error ? (
        <p
          role="alert"
          className="text-destructive text-sm"
          data-testid="bulk-level-error"
        >
          {t(`warehouse.bulk.error.${error.key}`, {
            code: "code" in error ? error.code : "",
          })}
        </p>
      ) : (
        <p
          className="text-muted-foreground text-sm"
          data-testid="bulk-level-count"
        >
          <Badge variant="secondary" className="me-2">
            {result.ok ? result.codes.length : 0}
          </Badge>
          <span className="font-mono text-xs" dir="ltr">
            {example}
          </span>
        </p>
      )}
    </fieldset>
  );
}

function Field({
  id,
  label,
  hint,
  value,
  onChange,
  inputMode,
  testId,
}: {
  id: string;
  label: string;
  hint?: string;
  value: string;
  onChange: (value: string) => void;
  inputMode?: "numeric";
  testId: string;
}) {
  return (
    <div className="space-y-1">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={value}
        inputMode={inputMode}
        autoComplete="off"
        dir="ltr"
        aria-describedby={hint ? `${id}-hint` : undefined}
        onChange={(e) => onChange(e.target.value)}
        data-testid={testId}
      />
      {hint ? (
        <p id={`${id}-hint`} className="text-muted-foreground text-xs">
          {hint}
        </p>
      ) : null}
    </div>
  );
}
