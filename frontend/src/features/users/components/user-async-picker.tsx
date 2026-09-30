"use client";

import { useCallback, useState } from "react";

import { AppCombobox, type ComboboxOption } from "@/components/forms";
import { userFullName } from "@/features/users/lib/user-display";
import { usersService } from "@/features/users/services/users.service";
import { useLocale } from "@/providers/locale-provider";

type UserAsyncPickerProps = {
  name: string;
  label?: string;
  description?: string;
  className?: string;
  disabled?: boolean;
};

/**
 * Async user search for forms — `value` is user UUID.
 * Uses `GET /v1/platform/users?q=&status=active&limit=20`.
 */
export function UserAsyncPicker({
  name,
  label,
  description,
  className,
  disabled,
}: UserAsyncPickerProps) {
  const { t } = useLocale();
  const [optionCache, setOptionCache] = useState<ComboboxOption[]>([]);

  const loadOptions = useCallback(async (query: string) => {
    const result = await usersService.list({
      limit: 20,
      offset: 0,
      q: query.trim() || undefined,
      status: "active",
    });
    const options = result.items.map((user): ComboboxOption => ({
      value: user.uuid,
      label: `${userFullName(user)} · ${user.email}`,
    }));
    setOptionCache((prev) => mergeOptions(prev, options));
    return options;
  }, []);

  return (
    <AppCombobox
      name={name}
      label={label}
      description={description}
      className={className}
      disabled={disabled}
      loadOptions={loadOptions}
      options={optionCache}
      placeholder={t("users.picker.placeholder")}
      searchPlaceholder={t("users.picker.search")}
      emptyText={t("users.picker.empty")}
    />
  );
}

function mergeOptions(
  prev: ComboboxOption[],
  next: ComboboxOption[],
): ComboboxOption[] {
  const byValue = new Map(prev.map((opt) => [opt.value, opt]));
  for (const opt of next) {
    byValue.set(opt.value, opt);
  }
  return [...byValue.values()];
}
