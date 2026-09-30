import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

export function ContentArea({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <main className={cn("flex flex-1 flex-col gap-4", className)}>
      {children}
    </main>
  );
}
