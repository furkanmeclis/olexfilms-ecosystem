"use client";

import { useParams } from "next/navigation";

import { TaskDetailPage } from "@/features/tasks";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <TaskDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}
