"use client";

import { Clock, Globe, HardDrive, Share2, Star, Trash2 } from "lucide-react";

import { Progress } from "@/components/ui/progress";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { formatBytes } from "@/features/storage/lib/format";
import type { StorageUsage, StorageView } from "@/features/storage/types";
import { useLocale } from "@/providers/locale-provider";

const views: { id: StorageView; icon: typeof HardDrive; labelKey: string }[] = [
  { id: "all", icon: HardDrive, labelKey: "storage.all_files" },
  { id: "recent", icon: Clock, labelKey: "storage.recent" },
  { id: "shared", icon: Share2, labelKey: "storage.shared" },
  { id: "public", icon: Globe, labelKey: "storage.public" },
  { id: "starred", icon: Star, labelKey: "storage.starred" },
  { id: "trash", icon: Trash2, labelKey: "storage.trash" },
];

export function StorageSidebar({
  view,
  onViewChange,
  usage,
}: {
  view: StorageView;
  onViewChange: (view: StorageView) => void;
  usage?: StorageUsage;
}) {
  const { t } = useLocale();
  const used = usage?.used_bytes ?? 0;
  const quota = usage?.quota_bytes ?? 0;
  const pct = quota > 0 ? Math.min(100, Math.round((used / quota) * 100)) : 0;

  return (
    <aside className="bg-card flex h-full w-60 shrink-0 flex-col border-e">
      <ScrollArea className="flex-1 p-3">
        <p className="text-muted-foreground mb-2 px-2 text-xs font-medium tracking-wide uppercase">
          {t("storage.sidebar_nav")}
        </p>
        <nav className="space-y-0.5">
          {views.map((item) => {
            const Icon = item.icon;
            const active = view === item.id;
            return (
              <button
                key={item.id}
                type="button"
                onClick={() => onViewChange(item.id)}
                className={cn(
                  "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm",
                  active
                    ? "bg-accent text-accent-foreground font-medium"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                <Icon className="size-4" />
                {t(item.labelKey)}
              </button>
            );
          })}
        </nav>
      </ScrollArea>
      <div className="border-t p-4">
        <p className="mb-2 text-sm font-medium">{t("storage.storage")}</p>
        {quota > 0 ? (
          <>
            <Progress value={pct} className="h-2" />
            <p className="text-muted-foreground mt-2 text-xs">
              {formatBytes(used)} / {formatBytes(quota)} · {pct}%
            </p>
          </>
        ) : (
          <>
            <Progress value={used > 0 ? 12 : 0} className="h-2" />
            <p className="text-muted-foreground mt-2 text-xs">
              {t("storage.used_of", { used: formatBytes(used) })}
            </p>
            <p className="text-muted-foreground text-xs">
              {t("storage.unlimited")}
            </p>
          </>
        )}
        {usage ? (
          <p className="text-muted-foreground mt-2 text-xs">
            {usage.total_files} {t("storage.total_files").toLowerCase()} ·{" "}
            {usage.total_folders} {t("storage.total_folders").toLowerCase()}
          </p>
        ) : null}
      </div>
    </aside>
  );
}
