"use client";

import { Bot } from "lucide-react";

import { PortalPage } from "@/features/portal/components/portal-page";
import { useLocale } from "@/providers/locale-provider";

import {
  PORTAL_ASSISTANT_SCOPE,
  portalTransport,
} from "../lib/portal-transport";
import { AssistantChat } from "./assistant-chat";

/**
 * Portal assistant (`/portal/assistant`): the panel chat with the customer
 * realm transport. Tool activity is not shown to customers.
 */
export function PortalAssistantPage() {
  const { t } = useLocale();
  return (
    <PortalPage
      title={t("ai.page.title")}
      icon={<Bot className="size-5" />}
      testId="portal-assistant"
    >
      <AssistantChat
        transport={portalTransport}
        scope={PORTAL_ASSISTANT_SCOPE}
        layout="page"
        showToolChips={false}
      />
    </PortalPage>
  );
}
