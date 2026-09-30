"use client";

import { createContext, useContext, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";

import {
  organizationsService,
  type PublicOrganization,
} from "@/features/organizations/services/organizations.service";

type TenantContextValue = {
  slug: string;
  organization: PublicOrganization | undefined;
  isLoading: boolean;
};

const TenantContext = createContext<TenantContextValue | null>(null);

export function TenantProvider({
  slug,
  children,
}: {
  slug: string;
  children: ReactNode;
}) {
  const query = useQuery({
    queryKey: ["organization", "public", slug],
    queryFn: () => organizationsService.getPublicBySlug(slug),
    staleTime: 60_000,
  });

  return (
    <TenantContext.Provider
      value={{
        slug,
        organization: query.data,
        isLoading: query.isLoading,
      }}
    >
      {children}
    </TenantContext.Provider>
  );
}

export function useOptionalTenant() {
  return useContext(TenantContext);
}

export function useTenant() {
  const ctx = useOptionalTenant();
  if (!ctx) {
    throw new Error("useTenant must be used within TenantProvider");
  }
  return ctx;
}
