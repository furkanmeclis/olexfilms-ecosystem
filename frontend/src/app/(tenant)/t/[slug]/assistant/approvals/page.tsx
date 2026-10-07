"use client";

import { useParams } from "next/navigation";
import { Suspense } from "react";

import { PendingActionsPage } from "@/features/mcp";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return (
    <Suspense>
      <PendingActionsPage slug={String(params.slug ?? "")} />
    </Suspense>
  );
}
