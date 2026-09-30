import { TableIcon } from "lucide-react";

import { useToolbarContext } from "@/components/editor/context/toolbar-context";
import { InsertTableDialog } from "@/components/editor/plugins/table-plugin";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { useLocale } from "@/providers/locale-provider";

export function InsertTable() {
  const { t } = useLocale();
  const { activeEditor, showModal } = useToolbarContext();

  return (
    <DropdownMenuItem
      onClick={() =>
        showModal(t("editor.insert.table_title"), (onClose) => (
          <InsertTableDialog activeEditor={activeEditor} onClose={onClose} />
        ))
      }
    >
      <div className="flex items-center gap-1">
        <TableIcon className="size-4" />
        <span>{t("editor.insert.table")}</span>
      </div>
    </DropdownMenuItem>
  );
}
