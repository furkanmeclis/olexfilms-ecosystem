import { $createCodeNode } from "@lexical/code";
import { $setBlocksType } from "@lexical/selection";
import { $getSelection, $isRangeSelection } from "lexical";

import { CodeIcon } from "lucide-react";

import { ComponentPickerOption } from "@/components/editor/plugins/picker/component-picker-option";

export function CodePickerPlugin({ title }: { title: string }) {
  return new ComponentPickerOption(title, {
    icon: <CodeIcon className="size-4" />,
    keywords: ["javascript", "python", "js", "codeblock"],
    onSelect: (_, editor) =>
      editor.update(() => {
        const selection = $getSelection();

        if ($isRangeSelection(selection)) {
          if (selection.isCollapsed()) {
            $setBlocksType(selection, () => $createCodeNode());
          } else {
            const textContent = selection.getTextContent();
            const codeNode = $createCodeNode();
            selection.insertNodes([codeNode]);
            selection.insertRawText(textContent);
          }
        }
      }),
  });
}
