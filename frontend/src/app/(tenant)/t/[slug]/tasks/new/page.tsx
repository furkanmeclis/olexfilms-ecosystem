"use client";

import { useParams } from "next/navigation";

import { TaskFormPage } from "@/features/tasks";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <TaskFormPage slug={String(params.slug ?? "")} />;
}
