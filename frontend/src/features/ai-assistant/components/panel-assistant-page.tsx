"use client";

import { Bot } from "lucide-react";
import { useMemo } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { FeatureGuard } from "@/features/modules/components/feature-guard";
import { useLocale } from "@/providers/locale-provider";

import { createPanelTransport } from "../lib/panel-transport";
import { AssistantChat } from "./assistant-chat";

/** Full-page panel assistant (`/t/{slug}/assistant`). */
export function PanelAssistantPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const transport = useMemo(() => createPanelTransport(slug), [slug]);
  return (
    <FeatureGuard slug={slug} feature={AI_ASSISTANT_FEATURE}>
      <PageHeader
        title={t("ai.page.title")}
        description={t("ai.page.description")}
        icon={<Bot className="size-6" />}
      />
      <AssistantChat
        transport={transport}
        scope={`panel:${slug}`}
        layout="page"
        showToolChips
      />
    </FeatureGuard>
  );
}

/** Module key of the assistant (backend module catalog). */
export const AI_ASSISTANT_FEATURE = "ai_assistant";
