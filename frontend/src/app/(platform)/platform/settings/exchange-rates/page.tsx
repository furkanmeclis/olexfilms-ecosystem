import { PermissionGuard } from "@/components/common/permission-guard";
import { permissions } from "@/config/permissions";
import { ExchangeRatesPage } from "@/features/exchange-rates";

export default function Page() {
  return (
    <PermissionGuard permission={permissions.rates.read}>
      <ExchangeRatesPage />
    </PermissionGuard>
  );
}
