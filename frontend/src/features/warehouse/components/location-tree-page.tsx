"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderTree, Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Permission } from "@/config/permissions";
import { LabelButton } from "@/features/warehouse/components/label-button";
import { LocationTree } from "@/features/warehouse/components/location-tree";
import {
  NodeForm,
  type NodeKind,
} from "@/features/warehouse/components/node-form";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import type { NodeFormValues } from "@/features/warehouse/lib/forms";
import {
  allowedChildTypes,
  branchUuids,
  buildLocationTree,
  filterLocationTree,
  type LocationNode,
} from "@/features/warehouse/lib/tree";
import {
  locationLabelsPath,
  warehouseKeys,
  warehouseService,
  type Warehouse,
  type WarehouseLocation,
  type WarehouseLocationType,
  type WarehouseRoom,
} from "@/features/warehouse/services/warehouse.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { appToast } from "@/providers/toast-provider";

type Panel =
  | { kind: "warehouse"; mode: "create" }
  | { kind: "warehouse"; mode: "edit"; target: Warehouse }
  | { kind: "room"; mode: "create" }
  | { kind: "room"; mode: "edit"; target: WarehouseRoom }
  | {
      kind: "location";
      mode: "create";
      parent: WarehouseLocation | null;
      types: WarehouseLocationType[];
    }
  | { kind: "location"; mode: "edit"; target: WarehouseLocation };

/**
 * Warehouse > Locations (TEC-201): warehouses of the active organization,
 * their rooms and the aisle > shelf > bin tree of the selected room, with
 * create / edit / delete and the QR label sheet (TEC-202).
 */
export function LocationTreePage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const qc = useQueryClient();
  const { confirmDelete } = useDialogs();

  // The picked ids; the effective ones fall back to the first row.
  const [pickedWarehouse, setWarehouseUuid] = useState<string | null>(null);
  const [pickedRoom, setRoomUuid] = useState<string | null>(null);
  const [panel, setPanel] = useState<Panel | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [selected, setSelected] = useState<string | null>(null);
  const [filter, setFilter] = useState("");

  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
    enabled: access.allowed,
  });
  const warehouseList = useMemo(
    () => warehouses.data?.items ?? [],
    [warehouses.data],
  );
  const currentWarehouse =
    warehouseList.find((w) => w.uuid === pickedWarehouse) ??
    warehouseList[0] ??
    null;
  const warehouseUuid = currentWarehouse?.uuid ?? null;

  const rooms = useQuery({
    queryKey: warehouseKeys.rooms(warehouseUuid ?? ""),
    queryFn: () => warehouseService.listRooms(warehouseUuid ?? ""),
    enabled: access.allowed && Boolean(warehouseUuid),
  });
  const roomList = useMemo(() => rooms.data?.items ?? [], [rooms.data]);
  const currentRoom =
    roomList.find((r) => r.uuid === pickedRoom) ?? roomList[0] ?? null;
  const roomUuid = currentRoom?.uuid ?? null;

  const locations = useQuery({
    queryKey: warehouseKeys.locations(roomUuid ?? ""),
    queryFn: () => warehouseService.listLocations(roomUuid ?? ""),
    enabled: access.allowed && Boolean(roomUuid),
  });
  const tree = useMemo(
    () => buildLocationTree(locations.data?.items ?? []),
    [locations.data],
  );
  const visibleTree = useMemo(
    () => filterLocationTree(tree, filter),
    [tree, filter],
  );
  const effectiveExpanded = useMemo(
    () => (filter.trim() ? new Set(branchUuids(visibleTree)) : expanded),
    [filter, visibleTree, expanded],
  );

  const save = useMutation({
    mutationFn: async ({ p, v }: { p: Panel; v: NodeFormValues }) => {
      const name = v.name || undefined;
      switch (p.kind) {
        case "warehouse":
          return p.mode === "create"
            ? warehouseService.createWarehouse({
                code: v.code,
                name,
                address: v.address || null,
              })
            : warehouseService.updateWarehouse(p.target.uuid, {
                code: v.code,
                name: v.name,
                address: v.address || null,
                active: v.active,
              });
        case "room":
          return p.mode === "create"
            ? warehouseService.createRoom(warehouseUuid ?? "", {
                code: v.code,
                name,
              })
            : warehouseService.updateRoom(p.target.uuid, {
                code: v.code,
                name: v.name,
                active: v.active,
              });
        case "location":
          return p.mode === "create"
            ? warehouseService.createLocation({
                room_uuid: roomUuid ?? "",
                parent_uuid: p.parent?.uuid ?? null,
                type: v.type as WarehouseLocationType,
                code: v.code,
                name,
              })
            : warehouseService.updateLocation(p.target.uuid, {
                code: v.code,
                name: v.name,
                active: v.active,
              });
      }
    },
    onSuccess: async (result, { p }) => {
      if (p.kind === "warehouse") {
        await qc.invalidateQueries({ queryKey: warehouseKeys.warehouses });
        if (p.mode === "create" && result) setWarehouseUuid(result.uuid);
      } else if (p.kind === "room") {
        await qc.invalidateQueries({
          queryKey: warehouseKeys.rooms(warehouseUuid ?? ""),
        });
        if (p.mode === "create" && result) setRoomUuid(result.uuid);
      }
      if (p.kind !== "warehouse") {
        await qc.invalidateQueries({ queryKey: ["warehouse", "locations"] });
      }
      if (p.kind === "location" && p.mode === "create" && p.parent) {
        const parentUuid = p.parent.uuid;
        setExpanded((prev) => new Set(prev).add(parentUuid));
      }
      appToast.success(t("warehouse.form.saved"));
      setPanel(null);
    },
  });

  const remove = useMutation({
    mutationFn: async ({ kind, uuid }: { kind: NodeKind; uuid: string }) => {
      if (kind === "warehouse") return warehouseService.deleteWarehouse(uuid);
      if (kind === "room") return warehouseService.deleteRoom(uuid);
      return warehouseService.deleteLocation(uuid);
    },
    onSuccess: async (_r, { kind }) => {
      if (kind === "warehouse") {
        setWarehouseUuid(null);
        setRoomUuid(null);
        await qc.invalidateQueries({ queryKey: warehouseKeys.warehouses });
      } else if (kind === "room") {
        setRoomUuid(null);
        await qc.invalidateQueries({
          queryKey: warehouseKeys.rooms(warehouseUuid ?? ""),
        });
      } else {
        await qc.invalidateQueries({ queryKey: ["warehouse", "locations"] });
      }
      appToast.success(t("warehouse.form.deleted"));
    },
    onError: (err) => {
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error")));
    },
  });

  const askDelete = async (kind: NodeKind, uuid: string, code: string) => {
    const ok = await confirmDelete({
      title: t("warehouse.delete.title", { code }),
      description: t(`warehouse.delete.${kind}`),
      confirmLabel: t("warehouse.delete.confirm"),
    });
    if (ok) remove.mutate({ kind, uuid });
  };

  const toggle = (uuid: string) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(uuid)) next.delete(uuid);
      else next.add(uuid);
      return next;
    });

  const nodeActions = (node: LocationNode) => {
    const childTypes = allowedChildTypes(node.type);
    return (
      <>
        <LabelButton
          path={locationLabelsPath({ locations: [node.uuid] })}
          filename={`${node.full_code}.pdf`}
          size="icon-sm"
          variant="ghost"
        />
        {canWrite && childTypes.length > 0 ? (
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            aria-label={t("warehouse.tree.add_child", { code: node.code })}
            title={t("warehouse.tree.add_child", { code: node.code })}
            data-testid="location-add-child"
            onClick={() =>
              setPanel({
                kind: "location",
                mode: "create",
                parent: node,
                types: childTypes,
              })
            }
          >
            <Plus className="size-4" />
          </Button>
        ) : null}
        {canWrite ? (
          <>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              aria-label={t("warehouse.tree.edit", { code: node.code })}
              title={t("warehouse.tree.edit", { code: node.code })}
              onClick={() =>
                setPanel({ kind: "location", mode: "edit", target: node })
              }
            >
              <Pencil className="size-4" />
            </Button>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              aria-label={t("warehouse.tree.delete", { code: node.code })}
              title={t("warehouse.tree.delete", { code: node.code })}
              onClick={() =>
                void askDelete("location", node.uuid, node.full_code)
              }
            >
              <Trash2 className="size-4" />
            </Button>
          </>
        ) : null}
      </>
    );
  };

  const panelContext = (p: Panel): string | undefined => {
    if (p.kind === "room") return currentWarehouse?.code;
    if (p.kind === "location") {
      if (p.mode === "edit") return p.target.full_code;
      return p.parent
        ? p.parent.full_code
        : [currentWarehouse?.code, currentRoom?.code].filter(Boolean).join("-");
    }
    return undefined;
  };

  const panelInitial = (p: Panel) => {
    if (p.mode !== "edit") return undefined;
    return {
      code: p.target.code,
      name: p.target.name,
      address: p.kind === "warehouse" ? (p.target.address ?? "") : "",
      active: p.target.active,
    };
  };

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.locations.title")}
      description={t("warehouse.locations.description")}
      icon={<FolderTree className="size-6" />}
    >
      {panel ? (
        <NodeForm
          key={`${panel.kind}-${panel.mode}-${
            panel.mode === "edit"
              ? panel.target.uuid
              : panel.kind === "location"
                ? (panel.parent?.uuid ?? "root")
                : "new"
          }`}
          kind={panel.kind}
          mode={panel.mode}
          context={panelContext(panel)}
          initial={panelInitial(panel)}
          allowedTypes={
            panel.kind === "location" && panel.mode === "create"
              ? panel.types
              : []
          }
          pending={save.isPending}
          onSubmit={(v) => save.mutateAsync({ p: panel, v })}
          onCancel={() => setPanel(null)}
        />
      ) : null}

      <div className="grid gap-6 lg:grid-cols-[18rem_1fr]">
        <div className="space-y-6">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between gap-2">
              <CardTitle>{t("warehouse.locations.warehouses")}</CardTitle>
              {canWrite ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  data-testid="warehouse-new"
                  onClick={() =>
                    setPanel({ kind: "warehouse", mode: "create" })
                  }
                >
                  <Plus className="size-4" />
                  {t("warehouse.locations.new_warehouse")}
                </Button>
              ) : null}
            </CardHeader>
            <CardContent className="space-y-1">
              {warehouses.isError ? (
                <ErrorState
                  title={t("common.error_generic")}
                  onRetry={() => void warehouses.refetch()}
                  retryLabel={t("common.retry")}
                />
              ) : warehouses.isLoading ? (
                <p className="text-muted-foreground text-sm">
                  {t("warehouse.list.loading")}
                </p>
              ) : warehouseList.length === 0 ? (
                <p
                  className="text-muted-foreground text-sm"
                  data-testid="warehouses-empty"
                >
                  {t("warehouse.locations.no_warehouses")}
                </p>
              ) : (
                warehouseList.map((w) => (
                  <SideRow
                    key={w.uuid}
                    active={w.uuid === warehouseUuid}
                    code={w.code}
                    name={w.name}
                    inactive={!w.active}
                    testId="warehouse-row"
                    onSelect={() => {
                      setWarehouseUuid(w.uuid);
                      setRoomUuid(null);
                    }}
                    onEdit={
                      canWrite
                        ? () =>
                            setPanel({
                              kind: "warehouse",
                              mode: "edit",
                              target: w,
                            })
                        : undefined
                    }
                    onDelete={
                      canWrite
                        ? () => void askDelete("warehouse", w.uuid, w.code)
                        : undefined
                    }
                  />
                ))
              )}
            </CardContent>
          </Card>

          {currentWarehouse ? (
            <Card>
              <CardHeader className="flex flex-row items-center justify-between gap-2">
                <CardTitle>{t("warehouse.locations.rooms")}</CardTitle>
                {canWrite ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    data-testid="room-new"
                    onClick={() => setPanel({ kind: "room", mode: "create" })}
                  >
                    <Plus className="size-4" />
                    {t("warehouse.locations.new_room")}
                  </Button>
                ) : null}
              </CardHeader>
              <CardContent className="space-y-1">
                {rooms.isLoading ? (
                  <p className="text-muted-foreground text-sm">
                    {t("warehouse.list.loading")}
                  </p>
                ) : roomList.length === 0 ? (
                  <p
                    className="text-muted-foreground text-sm"
                    data-testid="rooms-empty"
                  >
                    {t("warehouse.locations.no_rooms")}
                  </p>
                ) : (
                  roomList.map((r) => (
                    <SideRow
                      key={r.uuid}
                      active={r.uuid === roomUuid}
                      code={r.code}
                      name={r.name}
                      inactive={!r.active}
                      testId="room-row"
                      onSelect={() => setRoomUuid(r.uuid)}
                      onEdit={
                        canWrite
                          ? () =>
                              setPanel({
                                kind: "room",
                                mode: "edit",
                                target: r,
                              })
                          : undefined
                      }
                      onDelete={
                        canWrite
                          ? () => void askDelete("room", r.uuid, r.code)
                          : undefined
                      }
                    />
                  ))
                )}
              </CardContent>
            </Card>
          ) : null}
        </div>

        <Card>
          <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
            <CardTitle>
              {currentRoom
                ? t("warehouse.locations.tree_of", {
                    code: `${currentWarehouse?.code ?? ""}-${currentRoom.code}`,
                  })
                : t("warehouse.locations.tree")}
            </CardTitle>
            {currentRoom ? (
              <div className="flex flex-wrap gap-2">
                <LabelButton
                  path={locationLabelsPath({ room: currentRoom.uuid })}
                  filename={`${currentWarehouse?.code ?? "room"}-${currentRoom.code}-labels.pdf`}
                  label={t("warehouse.labels.print_room")}
                  testId="room-labels"
                />
                {canWrite ? (
                  <Button
                    type="button"
                    size="sm"
                    data-testid="location-new-root"
                    onClick={() =>
                      setPanel({
                        kind: "location",
                        mode: "create",
                        parent: null,
                        types: allowedChildTypes(null),
                      })
                    }
                  >
                    <Plus className="size-4" />
                    {t("warehouse.locations.new_root")}
                  </Button>
                ) : null}
              </div>
            ) : null}
          </CardHeader>
          <CardContent className="space-y-4">
            {!currentRoom ? (
              <p className="text-muted-foreground text-sm">
                {t("warehouse.locations.pick_room")}
              </p>
            ) : locations.isError ? (
              <ErrorState
                title={t("common.error_generic")}
                onRetry={() => void locations.refetch()}
                retryLabel={t("common.retry")}
              />
            ) : locations.isLoading ? (
              <p className="text-muted-foreground text-sm">
                {t("warehouse.list.loading")}
              </p>
            ) : tree.length === 0 ? (
              <p
                className="text-muted-foreground text-sm"
                data-testid="locations-empty"
              >
                {t("warehouse.locations.no_locations")}
              </p>
            ) : (
              <>
                <div className="flex flex-wrap items-center gap-2">
                  <Input
                    value={filter}
                    onChange={(e) => setFilter(e.target.value)}
                    placeholder={t("warehouse.tree.filter")}
                    aria-label={t("warehouse.tree.filter")}
                    className="max-w-xs"
                    data-testid="location-filter"
                  />
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => setExpanded(new Set(branchUuids(tree)))}
                  >
                    {t("warehouse.tree.expand_all")}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => setExpanded(new Set())}
                  >
                    {t("warehouse.tree.collapse_all")}
                  </Button>
                </div>
                {visibleTree.length === 0 ? (
                  <p className="text-muted-foreground text-sm">
                    {t("warehouse.tree.no_match")}
                  </p>
                ) : (
                  <LocationTree
                    label={t("warehouse.locations.tree")}
                    nodes={visibleTree}
                    expanded={effectiveExpanded}
                    onToggle={toggle}
                    selected={selected}
                    onSelect={(n) => setSelected(n.uuid)}
                    renderActions={nodeActions}
                  />
                )}
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </WarehouseShell>
  );
}

function SideRow({
  active,
  code,
  name,
  inactive,
  testId,
  onSelect,
  onEdit,
  onDelete,
}: {
  active: boolean;
  code: string;
  name: string;
  inactive: boolean;
  testId: string;
  onSelect: () => void;
  onEdit?: () => void;
  onDelete?: () => void;
}) {
  const { t } = useLocale();
  return (
    <div
      className={cn(
        "hover:bg-accent/50 flex items-center gap-1 rounded-md px-2 py-1",
        active && "bg-accent",
        inactive && "opacity-60",
      )}
      data-testid={testId}
      data-code={code}
    >
      <button
        type="button"
        className="flex min-w-0 flex-1 items-baseline gap-2 text-start"
        aria-pressed={active}
        onClick={onSelect}
      >
        <span className="font-mono text-sm font-medium" dir="ltr">
          {code}
        </span>
        {name && name !== code ? (
          <span className="text-muted-foreground truncate text-xs">{name}</span>
        ) : null}
      </button>
      {onEdit ? (
        <Button
          type="button"
          size="icon-xs"
          variant="ghost"
          aria-label={t("warehouse.tree.edit", { code })}
          onClick={onEdit}
        >
          <Pencil className="size-3.5" />
        </Button>
      ) : null}
      {onDelete ? (
        <Button
          type="button"
          size="icon-xs"
          variant="ghost"
          aria-label={t("warehouse.tree.delete", { code })}
          onClick={onDelete}
        >
          <Trash2 className="size-3.5" />
        </Button>
      ) : null}
    </div>
  );
}
