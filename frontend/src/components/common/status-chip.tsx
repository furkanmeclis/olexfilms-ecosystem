import { Badge } from "@/components/ui/badge";

const toneMap = {
  default: "secondary",
  success: "success",
  warning: "warning",
  danger: "danger",
} as const;

type StatusChipProps = {
  label: string;
  tone?: keyof typeof toneMap;
};

export function StatusChip({ label, tone = "default" }: StatusChipProps) {
  return <Badge variant={toneMap[tone]}>{label}</Badge>;
}
