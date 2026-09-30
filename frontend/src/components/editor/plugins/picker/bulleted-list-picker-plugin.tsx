import { INSERT_UNORDERED_LIST_COMMAND } from "@lexical/list";

import { ListIcon } from "lucide-react";

import { ComponentPickerOption } from "@/components/editor/plugins/picker/component-picker-option";

export function BulletedListPickerPlugin({ title }: { title: string }) {
  return new ComponentPickerOption(title, {
    icon: <ListIcon className="size-4" />,
    keywords: ["bulleted list", "unordered list", "ul"],
    onSelect: (_, editor) =>
      editor.dispatchCommand(INSERT_UNORDERED_LIST_COMMAND, undefined),
  });
}
