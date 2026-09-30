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

const BLOCK_FORMAT_VALUE = "paragraph";

export function FormatParagraph() {
  const { t } = useLocale();
  const { activeEditor } = useToolbarContext();

  const formatParagraph = () => {
    activeEditor.update(() => {
      const selection = $getSelection();
      if ($isRangeSelection(selection)) {
        $setBlocksType(selection, () => $createParagraphNode());
      }
    });
  };

  return (
    <DropdownMenuItem onClick={formatParagraph}>
      <div className="flex items-center gap-1 font-normal">
        {blockTypeToBlockName[BLOCK_FORMAT_VALUE].icon}
        {t(blockTypeToBlockName[BLOCK_FORMAT_VALUE].labelKey)}
      </div>
    </DropdownMenuItem>
  );
}
