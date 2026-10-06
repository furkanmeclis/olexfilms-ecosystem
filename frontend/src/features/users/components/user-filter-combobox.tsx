"use client";

import { useCallback, useState } from "react";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { userFullName } from "@/features/users/lib/user-display";
import { usersService } from "@/features/users/services/users.service";
import { useLocale } from "@/providers/locale-provider";

type UserFilterComboboxProps = {
  /** Selected user uuid ("" = none). */
  value: string;
  onValueChange: (uuid: string) => void;
  placeholder?: string;
  className?: string;
};

/**
 * Toolbar user filter (not a form field): async search over
 * `GET /v1/platform/users?q=`; the value is the user uuid.
 */
export function UserFilterCombobox({
  value,
  onValueChange,
  placeholder,
  className,
}: UserFilterComboboxProps) {
  const { t } = useLocale();
  // Keeps the label of the selected user once a later search replaces
  // the result list.
  const [seen, setSeen] = useState<ComboboxOption[]>([]);

  const loadOptions = useCallback(async (query: string) => {
    const result = await usersService.list({
      limit: 20,
      offset: 0,
      q: query.trim() || undefined,
    });
    const options = result.items.map((user): ComboboxOption => ({
      value: user.uuid,
      label: `${userFullName(user)} · ${user.email}`,
    }));
    setSeen((prev) => {
      const byValue = new Map(prev.map((opt) => [opt.value, opt]));
      for (const opt of options) byValue.set(opt.value, opt);
      return [...byValue.values()];
    });
    return options;
  }, []);

  return (
    <AsyncCombobox
      value={value}
      onValueChange={onValueChange}
      loadOptions={loadOptions}
      options={seen}
      placeholder={placeholder ?? t("users.picker.placeholder")}
      searchPlaceholder={t("users.picker.search")}
      emptyText={t("users.picker.empty")}
      clearable
      className={className}
    />
  );
}
