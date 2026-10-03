"use client";

import { useState } from "react";

import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { NativeSelectField } from "@/features/warehouse/components/native-select-field";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  nodeFormSchema,
  type NodeFormInput,
  type NodeFormValues,
} from "@/features/warehouse/lib/forms";
import type { WarehouseLocationType } from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

export type NodeKind = "warehouse" | "room" | "location";

type NodeFormProps = {
  kind: NodeKind;
  mode: "create" | "edit";
  /** Shown under the title, e.g. the parent's full code. */
  context?: string;
  initial?: Partial<NodeFormInput>;
  /** Location create: the types the parent permits (allowedChildTypes). */
  allowedTypes?: WarehouseLocationType[];
  pending?: boolean;
  onSubmit: (values: NodeFormValues) => Promise<unknown>;
  onCancel: () => void;
};

/**
 * Create / edit form of a warehouse, a room or a location (TEC-201). The
 * code is upper-cased (A-Z, 0-9, _; 1-32) like the backend; the code of a
 * location is unique among its siblings and part of its full_code.
 */
export function NodeForm({
  kind,
  mode,
  context,
  initial,
  allowedTypes = [],
  pending,
  onSubmit,
  onCancel,
}: NodeFormProps) {
  const { t } = useLocale();
  const [serverError, setServerError] = useState<string | null>(null);
  const typeChoices = mode === "create" && kind === "location";
  const schema = nodeFormSchema(t, typeChoices ? allowedTypes : []);

  return (
    <Card data-testid="node-form" data-kind={kind} data-mode={mode}>
      <CardHeader>
        <CardTitle>{t(`warehouse.form.${mode}_${kind}`)}</CardTitle>
        {context ? (
          <p className="text-muted-foreground font-mono text-xs" dir="ltr">
            {context}
          </p>
        ) : null}
      </CardHeader>
      <CardContent>
        <AppForm<NodeFormValues>
          schema={schema as never}
          defaultValues={{
            code: "",
            name: "",
            address: "",
            type: allowedTypes[0] ?? "",
            active: true,
            ...initial,
          }}
          onSubmit={async (values) => {
            setServerError(null);
            try {
              await onSubmit(values);
            } catch (err) {
              setServerError(
                warehouseErrorMessage(err, t, t("warehouse.form.error")),
              );
            }
          }}
          className="space-y-4"
        >
          {typeChoices ? (
            <NativeSelectField
              name="type"
              label={t("warehouse.fields.type")}
              options={allowedTypes.map((ty) => ({
                value: ty,
                label: t(`warehouse.type.${ty}`),
              }))}
              testId="node-type"
            />
          ) : null}
          <AppInput
            name="code"
            label={t("warehouse.fields.code")}
            description={t("warehouse.form.code_hint")}
            autoComplete="off"
            dir="ltr"
            data-testid="node-code"
          />
          <AppInput
            name="name"
            label={t("warehouse.fields.name")}
            description={t("warehouse.form.name_hint")}
            data-testid="node-name"
          />
          {kind === "warehouse" ? (
            <AppInput
              name="address"
              label={t("warehouse.fields.address")}
              data-testid="node-address"
            />
          ) : null}
          {mode === "edit" ? (
            <AppSwitch
              name="active"
              label={t("warehouse.fields.active")}
              description={t("warehouse.form.active_hint")}
            />
          ) : null}
          {serverError ? (
            <p
              role="alert"
              className="text-destructive text-sm"
              data-testid="node-form-error"
            >
              {serverError}
            </p>
          ) : null}
          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("warehouse.form.cancel")}
            </Button>
            <Button type="submit" disabled={pending} data-testid="node-save">
              {t("warehouse.form.save")}
            </Button>
          </div>
        </AppForm>
      </CardContent>
    </Card>
  );
}
