import { INSERT_CHECK_LIST_COMMAND } from "@lexical/list";
import { $setBlocksType } from "@lexical/selection";
import {
  $createParagraphNode,
  $getSelection,
  $isRangeSelection,
} from "lexical";

import { useToolbarContext } from "@/components/editor/context/toolbar-context";
import { blockTypeToBlockName } from "@/components/editor/plugins/toolbar/block-format/block-format-data";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { useLocale } from "@/providers/locale-provider";

const BLOCK_FORMAT_VALUE = "check";

export function FormatCheckList() {
  const { t } = useLocale();
  const { activeEditor, blockType } = useToolbarContext();

  const formatParagraph = () => {
    activeEditor.update(() => {
      const selection = $getSelection();
      if ($isRangeSelection(selection)) {
        $setBlocksType(selection, () => $createParagraphNode());
      }
    });
  };

  const formatCheckList = () => {
    if (blockType !== "check") {
      activeEditor.dispatchCommand(INSERT_CHECK_LIST_COMMAND, undefined);
    } else {
      formatParagraph();
    }
  };

  return (
    <DropdownMenuItem onClick={formatCheckList}>
      <div className="flex items-center gap-1 font-normal">
        {blockTypeToBlockName[BLOCK_FORMAT_VALUE].icon}
        {t(blockTypeToBlockName[BLOCK_FORMAT_VALUE].labelKey)}
      </div>
    </DropdownMenuItem>
  );
}
