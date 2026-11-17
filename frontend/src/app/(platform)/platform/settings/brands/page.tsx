import { PermissionGuard } from "@/components/common/permission-guard";
import { permissions } from "@/config/permissions";
import { BrandCurrencyPage } from "@/features/brand-currency";

export default function Page() {
  return (
    <PermissionGuard permission={permissions.organizations.read}>
      <BrandCurrencyPage />
    </PermissionGuard>
  );
}
