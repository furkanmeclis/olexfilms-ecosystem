"use client";

import { AppForm, AppInput, AppSwitch, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  categoryDefaults,
  categoryFormSchema,
  categoryInput,
  type CategoryFormValues,
} from "@/features/catalog/lib/form";
import type {
  CatalogCategory,
  CatalogCategoryInput,
} from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

type CategoryFormDialogProps = {
  open: boolean;
  category: CatalogCategory | null;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: CatalogCategoryInput) => Promise<unknown>;
};

/** Create / edit a product category, available_parts one per line. */
export function CategoryFormDialog({
  open,
  category,
  pending,
  onOpenChange,
  onSubmit,
}: CategoryFormDialogProps) {
  const { t } = useLocale();
  const schema = categoryFormSchema(t);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {category
              ? t("catalog.categories.edit_title")
              : t("catalog.categories.create_title")}
          </DialogTitle>
          <DialogDescription>
            {t("catalog.categories.form_description")}
          </DialogDescription>
        </DialogHeader>
        {open ? (
          <AppForm<CategoryFormValues>
            key={category?.uuid ?? "new"}
            schema={schema}
            defaultValues={categoryDefaults(category)}
            onSubmit={async (values) => {
              await onSubmit(categoryInput(values));
            }}
            className="space-y-4"
          >
            <AppInput name="name" label={t("catalog.fields.name")} />
            <AppTextarea
              name="available_parts"
              label={t("catalog.fields.available_parts")}
              description={t("catalog.categories.parts_hint")}
              rows={5}
            />
            <AppInput
              name="sort"
              inputMode="numeric"
              label={t("catalog.fields.sort")}
            />
            <AppSwitch name="active" label={t("catalog.fields.active")} />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={pending}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </AppForm>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
