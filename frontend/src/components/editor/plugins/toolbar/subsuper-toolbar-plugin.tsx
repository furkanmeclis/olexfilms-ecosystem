import { useState } from "react";

import { $isTableSelection } from "@lexical/table";
import {
  $isRangeSelection,
  type BaseSelection,
  FORMAT_TEXT_COMMAND,
} from "lexical";

import { SubscriptIcon, SuperscriptIcon } from "lucide-react";

import { useToolbarContext } from "@/components/editor/context/toolbar-context";
import { useUpdateToolbarHandler } from "@/components/editor/editor-hooks/use-update-toolbar";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useLocale } from "@/providers/locale-provider";

export function SubSuperToolbarPlugin() {
  const { t } = useLocale();
  const { activeEditor } = useToolbarContext();
  const [isSubscript, setIsSubscript] = useState(false);
  const [isSuperscript, setIsSuperscript] = useState(false);

  const $updateToolbar = (selection: BaseSelection) => {
    if ($isRangeSelection(selection) || $isTableSelection(selection)) {
      setIsSubscript(
        (selection as { hasFormat: (format: string) => boolean }).hasFormat(
          "subscript",
        ),
      );
      setIsSuperscript(
        (selection as { hasFormat: (format: string) => boolean }).hasFormat(
          "superscript",
        ),
      );
    }
  };

  useUpdateToolbarHandler($updateToolbar);

  return (
    <ToggleGroup
      type="single"
      defaultValue={
        isSubscript ? "subscript" : isSuperscript ? "superscript" : ""
      }
    >
      <ToggleGroupItem
        value="subscript"
        size="sm"
        aria-label={t("editor.subscript")}
        onClick={() => {
          activeEditor.dispatchCommand(FORMAT_TEXT_COMMAND, "subscript");
        }}
        variant={"outline"}
      >
        <SubscriptIcon className="size-4" />
      </ToggleGroupItem>
      <ToggleGroupItem
        value="superscript"
        size="sm"
        aria-label={t("editor.superscript")}
        onClick={() => {
          activeEditor.dispatchCommand(FORMAT_TEXT_COMMAND, "superscript");
        }}
        variant={"outline"}
      >
        <SuperscriptIcon className="size-4" />
      </ToggleGroupItem>
    </ToggleGroup>
  );
}
