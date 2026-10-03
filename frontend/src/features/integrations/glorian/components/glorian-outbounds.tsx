"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, RotateCcw } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import {
  OutboundStateBadge,
  selectClass,
} from "@/features/integrations/glorian/components/glorian-status";
import { glorianErrorText } from "@/features/integrations/glorian/lib/errors";
import { glorianKeys } from "@/features/integrations/glorian/lib/keys";
import {
  glorianService,
  type GlorianOutbound,
  type GlorianOutboundState,
} from "@/features/integrations/glorian/services/glorian.service";
import { useFormatter, useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const OUTBOUND_STATES: GlorianOutboundState[] = [
  "held",
  "failed",
  "pending",
  "sent",
  "cancelled",
];

/** Only held and failed outbounds can be replayed (TEC-273). */
export function isReplayable(o: Pick<GlorianOutbound, "state">) {
  return o.state === "held" || o.state === "failed";
}

/** Order outbounds of one state (held by default) with replay. */
export function GlorianOutbounds({ canManage }: { canManage: boolean }) {
  const { t } = useLocale();
  const format = useFormatter();
  const queryClient = useQueryClient();
  const [state, setState] = useState<GlorianOutboundState>("held");

  const list = useQuery({
    queryKey: glorianKeys.outbounds(state),
    queryFn: () => glorianService.listOutbounds(state),
  });

  const replay = useMutation({
    mutationFn: (uuid: string) => glorianService.replayOutbound(uuid),
    onSuccess: async (o) => {
      appToast.success(
        t("integrations.glorian.outbounds.replay_queued", {
          order: o.order_no,
        }),
      );
      await queryClient.invalidateQueries({
        queryKey: [...glorianKeys.all, "outbounds"],
      });
    },
    onError: (error) => appToast.error(glorianErrorText(t, error)),
  });

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end gap-3">
        <label className="grid gap-1 text-sm">
          <span>{t("integrations.glorian.outbounds.filter_state")}</span>
          <select
            className={selectClass}
            value={state}
            onChange={(e) => setState(e.target.value as GlorianOutboundState)}
            data-testid="outbounds-state"
          >
            {OUTBOUND_STATES.map((s) => (
              <option key={s} value={s}>
                {t(`integrations.glorian.outbound_state.${s}`)}
              </option>
            ))}
          </select>
        </label>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => list.refetch()}
          disabled={list.isFetching}
        >
          <RefreshCw className="size-4" aria-hidden />
          {t("integrations.glorian.refresh")}
        </Button>
      </div>

      {list.isLoading ? <Loading label={t("common.loading")} /> : null}
      {list.isError ? (
        <ErrorState
          title={glorianErrorText(t, list.error)}
          retryLabel={t("common.retry")}
          onRetry={() => list.refetch()}
        />
      ) : null}
      {list.data && list.data.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("integrations.glorian.outbounds.empty")}
        </p>
      ) : null}
      {list.data && list.data.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-sm" data-testid="outbounds">
            <thead className="text-muted-foreground text-xs">
              <tr className="border-b">
                <th className="p-2 text-start">
                  {t("integrations.glorian.outbounds.order")}
                </th>
                <th className="p-2 text-start">
                  {t("integrations.glorian.outbounds.reference")}
                </th>
                <th className="p-2 text-start">
                  {t("integrations.glorian.outbounds.state")}
                </th>
                <th className="p-2 text-start">
                  {t("integrations.glorian.outbounds.attempts")}
                </th>
                <th className="p-2 text-start">
                  {t("integrations.glorian.outbounds.updated")}
                </th>
                <th className="p-2 text-start">
                  {t("integrations.glorian.outbounds.reason")}
                </th>
                <th className="p-2" />
              </tr>
            </thead>
            <tbody>
              {list.data.map((o) => (
                <tr
                  key={o.uuid}
                  className="border-b align-top"
                  data-testid={`outbound-${o.uuid}`}
                >
                  <td className="p-2 font-mono">{o.order_no}</td>
                  <td className="p-2 font-mono">{o.external_reference}</td>
                  <td className="p-2">
                    <OutboundStateBadge state={o.state} />
                  </td>
                  <td className="p-2">{o.attempts}</td>
                  <td className="p-2 whitespace-nowrap">
                    {format.dateTime(o.updated_at)}
                  </td>
                  <td className="p-2">
                    {o.held_reason ? (
                      <div>
                        {t(`integrations.glorian.held_reason.${o.held_reason}`)}
                      </div>
                    ) : null}
                    {o.last_error ? (
                      <div className="text-destructive break-all">
                        {o.last_error}
                      </div>
                    ) : null}
                  </td>
                  <td className="p-2 text-end">
                    {canManage && isReplayable(o) ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => replay.mutate(o.uuid)}
                        disabled={replay.isPending}
                        data-testid={`replay-${o.uuid}`}
                      >
                        <RotateCcw className="size-4" aria-hidden />
                        {t("integrations.glorian.outbounds.replay")}
                      </Button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}
