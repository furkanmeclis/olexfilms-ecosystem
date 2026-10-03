"use client";

import { ChevronRight } from "lucide-react";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import type { LocationNode } from "@/features/warehouse/lib/tree";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type LocationTreeProps = {
  nodes: LocationNode[];
  expanded: ReadonlySet<string>;
  onToggle: (uuid: string) => void;
  selected?: string | null;
  onSelect?: (node: LocationNode) => void;
  /** Per-node controls (add child, edit, delete, print). */
  renderActions?: (node: LocationNode) => ReactNode;
  label: string;
};

/**
 * Location tree of one room (TEC-201): aisle > shelf > bin, siblings in
 * sort order. A WAI-ARIA tree: each row is a treeitem with its level and,
 * when it has children, its expanded state.
 */
export function LocationTree({
  nodes,
  expanded,
  onToggle,
  selected,
  onSelect,
  renderActions,
  label,
}: LocationTreeProps) {
  return (
    <ul role="tree" aria-label={label} className="space-y-0.5">
      {nodes.map((n) => (
        <TreeRow
          key={n.uuid}
          node={n}
          level={1}
          expanded={expanded}
          onToggle={onToggle}
          selected={selected}
          onSelect={onSelect}
          renderActions={renderActions}
        />
      ))}
    </ul>
  );
}

function TreeRow({
  node,
  level,
  expanded,
  onToggle,
  selected,
  onSelect,
  renderActions,
}: Omit<LocationTreeProps, "nodes" | "label"> & {
  node: LocationNode;
  level: number;
}) {
  const { t } = useLocale();
  const hasChildren = node.children.length > 0;
  const open = hasChildren && expanded.has(node.uuid);
  const isSelected = selected === node.uuid;

  return (
    <li
      role="treeitem"
      aria-level={level}
      aria-expanded={hasChildren ? open : undefined}
      aria-selected={isSelected}
      data-testid="location-node"
      data-uuid={node.uuid}
      data-code={node.full_code}
    >
      <div
        className={cn(
          "hover:bg-accent/50 flex items-center gap-2 rounded-md py-1 pe-1",
          isSelected && "bg-accent",
          !node.active && "opacity-60",
        )}
        style={{ paddingInlineStart: `${(level - 1) * 1.25}rem` }}
      >
        {hasChildren ? (
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground rounded p-0.5"
            aria-label={
              open ? t("warehouse.tree.collapse") : t("warehouse.tree.expand")
            }
            onClick={() => onToggle(node.uuid)}
            data-testid="location-toggle"
          >
            <ChevronRight
              className={cn(
                "size-4 transition-transform rtl:rotate-180",
                open && "rotate-90 rtl:rotate-90",
              )}
            />
          </button>
        ) : (
          <span className="inline-block size-5" aria-hidden />
        )}
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-2 text-start"
          onClick={() => onSelect?.(node)}
        >
          <Badge variant="secondary" className="shrink-0">
            {t(`warehouse.type.${node.type}`)}
          </Badge>
          <span className="font-mono text-sm font-medium" dir="ltr">
            {node.code}
          </span>
          {node.name && node.name !== node.code ? (
            <span className="text-muted-foreground truncate text-sm">
              {node.name}
            </span>
          ) : null}
          {!node.active ? (
            <span className="text-muted-foreground text-xs">
              {t("warehouse.tree.inactive")}
            </span>
          ) : null}
        </button>
        {renderActions ? (
          <div className="flex shrink-0 items-center gap-1">
            {renderActions(node)}
          </div>
        ) : null}
      </div>
      {open ? (
        <ul role="group" className="space-y-0.5">
          {node.children.map((c) => (
            <TreeRow
              key={c.uuid}
              node={c}
              level={level + 1}
              expanded={expanded}
              onToggle={onToggle}
              selected={selected}
              onSelect={onSelect}
              renderActions={renderActions}
            />
          ))}
        </ul>
      ) : null}
    </li>
  );
}
