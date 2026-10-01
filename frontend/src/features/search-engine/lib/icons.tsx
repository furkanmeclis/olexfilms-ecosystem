import {
  Activity,
  Bell,
  Car,
  Download,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  Package,
  Receipt,
  ScrollText,
  Settings2,
  Shield,
  Tags,
  Upload,
  Users,
  Wallet,
  type LucideIcon,
} from "lucide-react";

import { githubNavIcon } from "@/components/icons/github-icon";

const ICONS: Record<string, LucideIcon> = {
  users: Users,
  shield: Shield,
  pages: LayoutDashboard,
  bell: Bell,
  activity: Activity,
  logs: ScrollText,
  storage: HardDrive,
  access: KeyRound,
  imports: Upload,
  exports: Download,
  settings: Settings2,
  home: LayoutDashboard,
  github: githubNavIcon,
  car: Car,
  wallet: Wallet,
  tags: Tags,
  package: Package,
  receipt: Receipt,
  tenant_finance_accounts: Wallet,
  tenant_finance_categories: Tags,
  tenant_finance_transactions: Receipt,
};

export function resolveSearchIcon(name?: string): LucideIcon {
  if (!name) return LayoutDashboard;
  return ICONS[name.toLowerCase()] ?? LayoutDashboard;
}
