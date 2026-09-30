"use client";

import { QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { useState, type ReactNode } from "react";

import { createQueryClient } from "@/lib/query/client";

export function QueryProvider({ children }: { children: ReactNode }) {
  const [client] = useState(() => createQueryClient());
  const enableDevtools = process.env.NEXT_PUBLIC_ENABLE_DEVTOOLS === "true";

  return (
    <QueryClientProvider client={client}>
      {children}
      {enableDevtools ? (
        <ReactQueryDevtools
          initialIsOpen={false}
          buttonPosition="bottom-right"
        />
      ) : null}
    </QueryClientProvider>
  );
}
