import type { JSX } from "react";

import { INSERT_TABLE_COMMAND } from "@lexical/table";
import { type LexicalEditor } from "lexical";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { DialogFooter } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale } from "@/providers/locale-provider";

export function InsertTableDialog({
  activeEditor,
  onClose,
}: {
  activeEditor: LexicalEditor;
  onClose: () => void;
}): JSX.Element {
  const { t } = useLocale();
  const [rows, setRows] = useState("5");
  const [columns, setColumns] = useState("5");

  const isDisabled = useMemo(() => {
    const row = Number(rows);
    const column = Number(columns);
    return !(
      row &&
      row > 0 &&
      row <= 500 &&
      column &&
      column > 0 &&
      column <= 50
    );
  }, [rows, columns]);

  const onClick = () => {
    activeEditor.dispatchCommand(INSERT_TABLE_COMMAND, {
      columns,
      rows,
    });

    onClose();
  };

  return (
    <>
      <div className="grid gap-2">
        <Label htmlFor="table-modal-rows">{t("editor.table.rows")}</Label>
        <Input
          id="table-modal-rows"
          placeholder={t("editor.table.rows_placeholder")}
          onChange={(e) => setRows(e.target.value)}
          value={rows}
          data-test-id="table-modal-rows"
          type="number"
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="table-modal-columns">{t("editor.table.columns")}</Label>
        <Input
          id="table-modal-columns"
          placeholder={t("editor.table.cols_placeholder")}
          onChange={(e) => setColumns(e.target.value)}
          value={columns}
          data-test-id="table-modal-columns"
          type="number"
        />
      </div>
      <DialogFooter data-test-id="table-model-confirm-insert">
        <Button disabled={isDisabled} onClick={onClick}>
          {t("editor.table.confirm")}
        </Button>
      </DialogFooter>
    </>
  );
}
