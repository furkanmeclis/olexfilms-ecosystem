import { Columns3Icon } from "lucide-react";

import { useToolbarContext } from "@/components/editor/context/toolbar-context";
import { InsertLayoutDialog } from "@/components/editor/plugins/layout-plugin";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { useLocale } from "@/providers/locale-provider";

export function InsertColumnsLayout() {
  const { t } = useLocale();
  const { activeEditor, showModal } = useToolbarContext();

  return (
    <DropdownMenuItem
      onClick={() =>
        showModal(t("editor.insert.columns_title"), (onClose) => (
          <InsertLayoutDialog activeEditor={activeEditor} onClose={onClose} />
        ))
      }
    >
      <div className="flex items-center gap-1">
        <Columns3Icon className="size-4" />
        <span>{t("editor.insert.columns")}</span>
      </div>
    </DropdownMenuItem>
  );
}
