"use client";

import Link from "next/link";
import type { MouseEvent, ReactNode } from "react";

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { entityHref, type EntityKind } from "@/config/routes";
import { useOptionalTenant } from "@/features/organizations/providers/tenant-provider";
import { cn } from "@/lib/utils/index";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { canOpenEntity, entityKindLabelKey } from "./entity-registry";

/**
 * A reference to another record as the API returns it (TEC-1216 `Ref`):
 * `uuid` + display `name`, optional secondary line `sub`. For `stock_unit`
 * the id is the barcode (`uuid` may carry it, or `barcode`).
 */
export type EntityRefValue = {
  uuid?: string | null;
  name?: string | null;
  sub?: string | null;
  barcode?: string | null;
};

export type EntityLinkProps = {
  kind: EntityKind;
  /** The ref (`{uuid,name,sub?}`); null / missing renders `fallback`. */
  entity: EntityRefValue | null | undefined;
  /** Label override (default: `entity.name`, then `entity.sub`). */
  children?: ReactNode;
  /** Rendered when there is nothing to show (default "—"). */
  fallback?: ReactNode;
  /** Shows the kind and `sub` in a hover card. */
  hoverCard?: boolean;
  className?: string;
  /** Tenant slug override (default: the tenant shell's slug). */
  slug?: string;
};

/** Detail page id of a ref (stock units are addressed by barcode). */
export function entityRefId(
  kind: EntityKind,
  entity: EntityRefValue | null | undefined,
): string | null {
  if (!entity) return null;
  if (kind === "stock_unit") return entity.barcode || entity.uuid || null;
  return entity.uuid || null;
}

/** Display text of a ref (`name`, then `sub`, then the barcode). */
export function entityRefLabel(
  entity: EntityRefValue | null | undefined,
): string {
  if (!entity) return "";
  return (entity.name || entity.sub || entity.barcode || "").trim();
}

/**
 * Permission check of the shell, or null outside a `PermissionProvider`
 * (isolated component tests): links then degrade to plain text.
 */
export function useEntityPermissionCheck():
  ((permission: string) => boolean) | null {
  try {
    return usePermission().can;
  } catch {
    return null;
  }
}

/**
 * Returns the href a ref opens for the current user, or null: the kind has
 * no page, the id is missing or the user lacks the read permission.
 */
export function useEntityHref(
  kind: EntityKind,
  entity: EntityRefValue | null | undefined,
  slugOverride?: string,
): string | null {
  const tenant = useOptionalTenant();
  const can = useEntityPermissionCheck();
  const slug = slugOverride ?? tenant?.slug;
  if (!can || !canOpenEntity(kind, can)) return null;
  return entityHref(kind, entityRefId(kind, entity), slug);
}

const stopRowClick = (e: MouseEvent) => e.stopPropagation();

/**
 * TEC-1217: name of a related record, linked to its detail page. Plain text
 * when the user may not open it or the kind has no page yet.
 */
export function EntityLink({
  kind,
  entity,
  children,
  fallback = "—",
  hoverCard = false,
  className,
  slug,
}: EntityLinkProps) {
  const { t } = useLocale();
  const href = useEntityHref(kind, entity, slug);
  const label = children ?? entityRefLabel(entity);
  if (!label) return <>{fallback}</>;

  const node = href ? (
    <Link
      href={href}
      onClick={stopRowClick}
      data-entity-kind={kind}
      className={cn(
        "text-primary underline-offset-4 hover:underline focus-visible:underline",
        className,
      )}
    >
      {label}
    </Link>
  ) : (
    <span data-entity-kind={kind} className={className}>
      {label}
    </span>
  );

  if (!hoverCard) return node;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{node}</TooltipTrigger>
      <TooltipContent>
        <span className="block font-medium">{t(entityKindLabelKey(kind))}</span>
        <span className="block">{entityRefLabel(entity)}</span>
        {entity?.sub && entity.sub !== entity.name ? (
          <span className="block opacity-80">{entity.sub}</span>
        ) : null}
      </TooltipContent>
    </Tooltip>
  );
}
