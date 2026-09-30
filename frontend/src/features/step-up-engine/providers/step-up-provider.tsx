"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { StepUpDialog } from "@/features/step-up-engine/components/step-up-dialog";
import { stepUpService } from "@/features/step-up-engine/services/stepup.service";
import type { StepUpStatus } from "@/features/step-up-engine/types";
import {
  registerStepUpEnsure,
  unregisterStepUpEnsure,
} from "@/features/step-up-engine/lib/step-up-interceptor";
import { useAuth } from "@/providers/auth-provider";

const STATUS_KEY = ["auth", "step-up", "status"] as const;

type StepUpContextValue = {
  status: StepUpStatus | undefined;
  isEnsuring: boolean;
  refresh: () => Promise<StepUpStatus | undefined>;
  ensure: () => Promise<boolean>;
};

const StepUpContext = createContext<StepUpContextValue | null>(null);

export function StepUpProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const { isAuthenticated } = useAuth();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [isEnsuring, setIsEnsuring] = useState(false);
  const resolverRef = useRef<((ok: boolean) => void) | null>(null);

  const statusQuery = useQuery({
    queryKey: STATUS_KEY,
    queryFn: () => stepUpService.getStatus(),
    enabled: isAuthenticated,
    staleTime: 30_000,
  });

  const refresh = useCallback(async () => {
    const next = await queryClient.fetchQuery({
      queryKey: STATUS_KEY,
      queryFn: () => stepUpService.getStatus(),
    });
    return next;
  }, [queryClient]);

  const finishEnsure = useCallback(
    async (ok: boolean) => {
      if (ok) {
        await refresh();
      }
      resolverRef.current?.(ok);
      resolverRef.current = null;
      setIsEnsuring(false);
      setDialogOpen(false);
    },
    [refresh],
  );

  const ensure = useCallback(async () => {
    if (!isAuthenticated) {
      return false;
    }
    const current = statusQuery.data ?? (await refresh());
    if (current?.valid) {
      return true;
    }
    if (resolverRef.current) {
      return new Promise<boolean>((resolve) => {
        const previous = resolverRef.current;
        resolverRef.current = (ok) => {
          previous?.(ok);
          resolve(ok);
        };
      });
    }
    setIsEnsuring(true);
    setDialogOpen(true);
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
    });
  }, [isAuthenticated, refresh, statusQuery.data]);

  const contextValue = useMemo<StepUpContextValue>(
    () => ({
      status: statusQuery.data,
      isEnsuring,
      refresh,
      ensure,
    }),
    [ensure, isEnsuring, refresh, statusQuery.data],
  );

  useEffect(() => {
    registerStepUpEnsure(ensure);
    return () => unregisterStepUpEnsure();
  }, [ensure]);

  return (
    <StepUpContext.Provider value={contextValue}>
      {children}
      <StepUpDialog
        open={dialogOpen}
        status={statusQuery.data ?? null}
        onOpenChange={(open) => {
          if (!open && resolverRef.current) {
            void finishEnsure(false);
            return;
          }
          setDialogOpen(open);
        }}
        onVerified={() => finishEnsure(true)}
      />
    </StepUpContext.Provider>
  );
}

export function useStepUp() {
  const ctx = useContext(StepUpContext);
  if (!ctx) {
    throw new Error("useStepUp must be used within StepUpProvider");
  }
  return ctx;
}

export function invalidateStepUpStatus(
  queryClient: ReturnType<typeof useQueryClient>,
) {
  return queryClient.invalidateQueries({ queryKey: STATUS_KEY });
}
