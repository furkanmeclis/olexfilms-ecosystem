"use client";

import { ChevronRight } from "lucide-react";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import {
  SortableArea,
  SortableGroup,
  SortableRow,
} from "@/features/warehouse/components/sortable";
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
  /**
   * Drag and drop between siblings (TEC-764); receives the parent uuid
   * (null = room root) and the siblings' new uuid order.
   */
  onReorder?: (parentUuid: string | null, order: string[]) => void;
};

/** SortableGroup key of the room root's siblings. */
const ROOT_GROUP = "root";

/**
 * Location tree of one room (TEC-201): aisle > shelf > bin, siblings in
 * sort order. A WAI-ARIA tree: each row is a treeitem with its level and,
 * when it has children, its expanded state. With `onReorder` every row
 * gets a grip to move it among its siblings (TEC-764).
 */
export function LocationTree({
  nodes,
  expanded,
  onToggle,
  selected,
  onSelect,
  renderActions,
  label,
  onReorder,
}: LocationTreeProps) {
  const sortable = Boolean(onReorder);
  const tree = (
    <ul role="tree" aria-label={label} className="space-y-0.5">
      <SortableGroup group={ROOT_GROUP} ids={nodes.map((n) => n.uuid)}>
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
            sortable={sortable}
          />
        ))}
      </SortableGroup>
    </ul>
  );
  if (!onReorder) return tree;
  return (
    <SortableArea
      onMove={(group, order) =>
        onReorder(group === ROOT_GROUP ? null : group, order)
      }
    >
      {tree}
    </SortableArea>
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
  sortable,
}: Omit<LocationTreeProps, "nodes" | "label" | "onReorder"> & {
  node: LocationNode;
  level: number;
  sortable: boolean;
}) {
  const { t } = useLocale();
  const hasChildren = node.children.length > 0;
  const open = hasChildren && expanded.has(node.uuid);
  const isSelected = selected === node.uuid;

  return (
    <SortableRow
      id={node.uuid}
      disabled={!sortable}
      handleLabel={t("warehouse.sort.handle", { code: node.code })}
    >
      {({ setNodeRef, style, isDragging, handle }) => (
        <li
          ref={setNodeRef}
          style={style}
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
              isDragging && "bg-background relative z-10 shadow-sm",
            )}
            style={{ paddingInlineStart: `${(level - 1) * 1.25}rem` }}
          >
            {handle}
            {hasChildren ? (
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground rounded p-0.5"
                aria-label={
                  open
                    ? t("warehouse.tree.collapse")
                    : t("warehouse.tree.expand")
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
              data-testid="location-select"
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
              <SortableGroup
                group={node.uuid}
                ids={node.children.map((c) => c.uuid)}
              >
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
                    sortable={sortable}
                  />
                ))}
              </SortableGroup>
            </ul>
          ) : null}
        </li>
      )}
    </SortableRow>
  );
}
