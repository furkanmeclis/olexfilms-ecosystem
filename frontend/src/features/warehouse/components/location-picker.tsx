"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { Label } from "@/components/ui/label";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  buildLocationTree,
  type LocationNode,
} from "@/features/warehouse/lib/tree";
import {
  warehouseKeys,
  warehouseService,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

/** Flattened tree as indented select options. */
export function locationOptions(
  nodes: LocationNode[],
  depth = 0,
): { value: string; label: string }[] {
  return nodes.flatMap((n) => [
    {
      value: n.uuid,
      label: `${"  ".repeat(depth)}${n.full_code}${n.active ? "" : " ×"}`,
    },
    ...locationOptions(n.children, depth + 1),
  ]);
}

/**
 * Room, then location (searchable), of one warehouse.
 * `onRoom` reports the room so a room-scoped form can use it alone.
 */
export function LocationPicker({
  warehouseUuid,
  value,
  onChange,
  onRoom,
  testId,
  disabled,
  roomOnly,
}: {
  warehouseUuid: string;
  value: string;
  onChange: (locationUuid: string) => void;
  onRoom?: (roomUuid: string) => void;
  /** Prefix of the test ids: `<id>-room`, `<id>-location`. */
  testId: string;
  disabled?: boolean;
  /** Only the room select (a room-scoped count). */
  roomOnly?: boolean;
}) {
  const { t } = useLocale();
  const [roomUuid, setRoomUuid] = useState("");
  const rooms = useQuery({
    queryKey: warehouseKeys.rooms(warehouseUuid),
    queryFn: () => warehouseService.listRooms(warehouseUuid),
    enabled: Boolean(warehouseUuid),
  });
  const locations = useQuery({
    queryKey: warehouseKeys.locations(roomUuid),
    queryFn: () => warehouseService.listLocations(roomUuid),
    enabled: Boolean(roomUuid) && !roomOnly,
  });
  const options = useMemo(
    () => locationOptions(buildLocationTree(locations.data?.items ?? [])),
    [locations.data],
  );

  return (
    <div className="grid gap-2 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor={`${testId}-room`}>{t("warehouse.fields.room")}</Label>
        <Select
          value={roomUuid}
          disabled={disabled || !warehouseUuid}
          onValueChange={(next) => {
            setRoomUuid(next);
            onRoom?.(next);
            onChange("");
          }}
        >
          <SelectTrigger id={`${testId}-room`} data-testid={`${testId}-room`}>
            <SelectValue placeholder={t("warehouse.entry.pick_room")} />
          </SelectTrigger>
          <SelectContent>
            {(rooms.data?.items ?? []).map((r) => (
              <SelectItem key={r.uuid} value={r.uuid}>
                {r.code} · {r.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className={roomOnly ? "hidden" : "space-y-1.5"}>
        <Label htmlFor={`${testId}-location`}>
          {t("warehouse.fields.location")}
        </Label>
        <AsyncCombobox
          id={`${testId}-location`}
          data-testid={`${testId}-location`}
          value={value}
          disabled={disabled || !roomUuid}
          placeholder={t("warehouse.entry.pick_location")}
          options={options}
          onValueChange={onChange}
        />
      </div>
    </div>
  );
}
