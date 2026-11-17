"use client";

import { useState } from "react";

import {
  canOpenEntity,
  RelatedList,
  RELATED_LIST_SPECS,
} from "@/components/entity";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { EntityKind } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type CustomerRelatedTab = {
  kind: EntityKind;
  /** Reverse filter of the kind's list endpoint. */
  param: string;
  /**
   * Whether the list endpoint already accepts `param` (TEC-1216). An
   * endpoint that ignores an unknown filter would list every record, so a
   * tab stays hidden until its filter exists.
   */
  ready: boolean;
  hideColumns?: string[];
};

/** TEC-1217: reverse tabs of the customer card, in display order. */
export const CUSTOMER_RELATED_TABS: readonly CustomerRelatedTab[] = [
  {
    kind: "service",
    param: "customer_uuid",
    ready: true,
    hideColumns: ["customer"],
  },
  { kind: "warranty", param: "customer_uuid", ready: true },
  { kind: "appointment", param: "customer_uuid", ready: true },
  { kind: "lead", param: "customer_uuid", ready: true },
  { kind: "measurement", param: "customer_uuid", ready: true },
  { kind: "contract", param: "customer_uuid", ready: true },
];

/** Tabs the caller may open whose list endpoint and filter exist. */
export function visibleCustomerRelatedTabs(
  can: (permission: string) => boolean,
): CustomerRelatedTab[] {
  return CUSTOMER_RELATED_TABS.filter(
    (tab) =>
      tab.ready &&
      Boolean(RELATED_LIST_SPECS[tab.kind]) &&
      canOpenEntity(tab.kind, can),
  );
}

/**
 * TEC-1217: the customer's services, warranties, appointments, leads,
 * measurements and contracts (each from its own list endpoint with the
 * `customer_uuid` reverse filter).
 */
export function CustomerRelatedTabs({
  customerUuid,
}: {
  customerUuid: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const tabs = visibleCustomerRelatedTabs(can);
  const [active, setActive] = useState<string>(tabs[0]?.kind ?? "");
  if (tabs.length === 0) return null;
  const current = tabs.some((tab) => tab.kind === active)
    ? active
    : tabs[0].kind;
  return (
    <Card data-testid="customer-related">
      <CardHeader>
        <CardTitle className="text-base">
          {t("entity.related.section_title")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <Tabs value={current} onValueChange={setActive}>
          <TabsList className="flex-wrap">
            {tabs.map((tab) => (
              <TabsTrigger
                key={tab.kind}
                value={tab.kind}
                data-testid={`customer-related-tab-${tab.kind}`}
              >
                {t(`entity.related.tabs.${tab.kind}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          {tabs.map((tab) => (
            <TabsContent key={tab.kind} value={tab.kind} className="pt-4">
              {current === tab.kind ? (
                <RelatedList
                  kind={tab.kind}
                  filter={{ [tab.param]: customerUuid }}
                  persistKey={`customer-related-${tab.kind}-v1`}
                  hideColumns={tab.hideColumns}
                />
              ) : null}
            </TabsContent>
          ))}
        </Tabs>
      </CardContent>
    </Card>
  );
}
