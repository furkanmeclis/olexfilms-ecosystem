"use client";

import { Bot } from "lucide-react";
import { usePathname } from "next/navigation";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useFeature } from "@/features/modules/hooks/use-features";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { createPanelTransport } from "../lib/panel-transport";
import { AssistantChat } from "./assistant-chat";
import { AI_ASSISTANT_FEATURE } from "./panel-assistant-page";

/** Tenant slug of a `/t/{slug}/...` path. */
export function tenantSlugOf(pathname: string): string | null {
  const match = /^\/t\/([^/]+)/.exec(pathname);
  return match?.[1] ? decodeURIComponent(match[1]) : null;
}

/**
 * Header button that opens the assistant in a side sheet. Shown only under
 * a tenant route whose organization has the `ai_assistant` module on and a
 * user with `ai.use`.
 */
export function AssistantHeaderButton() {
  const pathname = usePathname() ?? "";
  const slug = tenantSlugOf(pathname);
  if (!slug || pathname === routes.tenant.login(slug)) return null;
  return <AssistantHeaderButtonFor slug={slug} />;
}

function AssistantHeaderButtonFor({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const { enabled } = useFeature(slug, AI_ASSISTANT_FEATURE);
  const [open, setOpen] = useState(false);
  const transport = useMemo(() => createPanelTransport(slug), [slug]);

  if (!enabled || !can(permissions.ai.use)) return null;
  const fullPage = routes.tenant.assistant(slug);

  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t("ai.nav_title")}
        title={t("ai.nav_title")}
        onClick={() => setOpen(true)}
        data-testid="ai-header-button"
      >
        <Bot className="size-5" />
      </Button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className="flex w-full flex-col gap-4 sm:max-w-lg">
          <SheetHeader>
            <SheetTitle>{t("ai.page.title")}</SheetTitle>
            <SheetDescription>{t("ai.page.description")}</SheetDescription>
          </SheetHeader>
          {open ? (
            <AssistantChat
              transport={transport}
              scope={`panel:${slug}`}
              layout="sheet"
              showToolChips
              fullPageHref={fullPage}
            />
          ) : null}
        </SheetContent>
      </Sheet>
    </>
  );
}
