"use client";

import { useParams } from "next/navigation";

import { TasksListPage } from "@/features/tasks";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <TasksListPage slug={String(params.slug ?? "")} />;
}
