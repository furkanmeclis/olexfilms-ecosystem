"use client";

import { useCallback, useState } from "react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { permissions } from "@/config/permissions";
import { userFullName } from "@/features/users/lib/user-display";
import { usersService } from "@/features/users/services/users.service";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export type NotificationAudienceScope = "me" | "all" | "user";

export type NotificationAudience = {
  scope: NotificationAudienceScope;
  userUuid?: string;
};

type NotificationAudienceFilterProps = {
  value: NotificationAudience;
  onChange: (next: NotificationAudience) => void;
};

export function NotificationAudienceFilter({
  value,
  onChange,
}: NotificationAudienceFilterProps) {
  const { t } = useLocale();
  const { user } = useAuth();
  const { can } = usePermission();
  const canReadAll =
    can(permissions.notifications.platformReadAll) ||
    Boolean(user?.isSuperAdmin);
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
    setOptionCache((prev) => {
      const byValue = new Map(prev.map((opt) => [opt.value, opt]));
      for (const opt of options) byValue.set(opt.value, opt);
      return [...byValue.values()];
    });
    return options;
  }, []);

  if (!canReadAll) {
    return (
      <div className="text-muted-foreground flex h-9 items-center rounded-md border px-3 text-sm">
        {t("notifications.audience.me")}
      </div>
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Select
        value={value.scope}
        onValueChange={(scope) => {
          const next = scope as NotificationAudienceScope;
          onChange({
            scope: next,
            userUuid: next === "user" ? value.userUuid : undefined,
          });
        }}
      >
        <SelectTrigger
          className="w-[220px]"
          aria-label={t("notifications.audience.label")}
        >
          <SelectValue placeholder={t("notifications.audience.label")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="me">{t("notifications.audience.me")}</SelectItem>
          <SelectItem value="all">{t("notifications.audience.all")}</SelectItem>
          <SelectItem value="user">
            {t("notifications.audience.user")}
          </SelectItem>
        </SelectContent>
      </Select>

      {value.scope === "user" ? (
        <AsyncCombobox
          className="w-[280px]"
          value={value.userUuid ?? ""}
          onValueChange={(userUuid) =>
            onChange({ scope: "user", userUuid: userUuid || undefined })
          }
          loadOptions={loadOptions}
          options={optionCache}
          clearable
          placeholder={t("notifications.audience.user_placeholder")}
          searchPlaceholder={t("notifications.audience.user_search")}
          emptyText={t("notifications.audience.user_empty")}
        />
      ) : null}
    </div>
  );
}
