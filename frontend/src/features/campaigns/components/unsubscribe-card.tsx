"use client";

import { CircleAlert, MailX, MailCheck } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { Button, buttonVariants } from "@/components/ui/button";
import {
  unsubscribeCampaignMessages,
  type UnsubscribeResult,
} from "@/features/campaigns/lib/unsubscribe";

export type UnsubscribeTexts = {
  title: string;
  body: string;
  confirm: string;
  done_title: string;
  done_body: string;
  invalid_title: string;
  invalid_body: string;
  error: string;
  home: string;
};

/**
 * Body of `/abonelik-iptal/{token}` (TEC-407): one explicit click writes
 * the marketing opt-out (link scanners only GET the page, so they never
 * unsubscribe anyone). Logical properties only (RTL for ar).
 */
export function UnsubscribeCard({
  token,
  texts,
  lang,
  dir,
  fetchImpl,
}: {
  token: string;
  texts: UnsubscribeTexts;
  lang: string;
  dir: "ltr" | "rtl";
  fetchImpl?: typeof fetch;
}) {
  const [state, setState] = useState<UnsubscribeResult | "idle" | "pending">(
    "idle",
  );
  const done = state === "done";
  const invalid = state === "invalid";
  const Icon = done ? MailCheck : invalid ? CircleAlert : MailX;

  async function submit() {
    setState("pending");
    setState(await unsubscribeCampaignMessages(token, fetchImpl));
  }

  return (
    <main
      lang={lang}
      dir={dir}
      data-slot="campaign-unsubscribe"
      className="bg-muted/30 text-foreground flex min-h-dvh items-center px-4 py-6"
    >
      <section
        data-state={state}
        className="bg-card mx-auto flex w-full max-w-md flex-col items-center gap-3 rounded-2xl border p-6 text-center shadow-sm"
      >
        <span className="bg-muted text-muted-foreground flex size-14 items-center justify-center rounded-full">
          <Icon className="size-8" aria-hidden />
        </span>
        <h1 className="text-lg font-semibold">
          {done
            ? texts.done_title
            : invalid
              ? texts.invalid_title
              : texts.title}
        </h1>
        <p className="text-muted-foreground text-sm" aria-live="polite">
          {done ? texts.done_body : invalid ? texts.invalid_body : texts.body}
        </p>
        {state === "error" ? (
          <p role="alert" className="text-destructive text-sm">
            {texts.error}
          </p>
        ) : null}
        {done || invalid ? (
          <Link href="/" className={buttonVariants({ variant: "outline" })}>
            {texts.home}
          </Link>
        ) : (
          <Button type="button" onClick={submit} disabled={state === "pending"}>
            {texts.confirm}
          </Button>
        )}
      </section>
    </main>
  );
}
