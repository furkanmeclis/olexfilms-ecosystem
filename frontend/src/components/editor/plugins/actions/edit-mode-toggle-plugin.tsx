import { useState } from "react";

import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";

import { LockIcon, UnlockIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useLocale } from "@/providers/locale-provider";

export function EditModeTogglePlugin() {
  const { t } = useLocale();
  const [editor] = useLexicalComposerContext();
  const [isEditable, setIsEditable] = useState(() => editor.isEditable());

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant={"ghost"}
          onClick={() => {
            editor.setEditable(!editor.isEditable());
            setIsEditable(editor.isEditable());
          }}
          title={
            isEditable ? t("editor.lock_readonly") : t("editor.unlock_readonly")
          }
          aria-label={
            isEditable ? t("editor.lock_readonly") : t("editor.unlock_readonly")
          }
          size={"sm"}
          className="p-2"
        >
          {isEditable ? (
            <LockIcon className="size-4" />
          ) : (
            <UnlockIcon className="size-4" />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {isEditable ? t("editor.view_only_mode") : t("editor.edit_mode")}
      </TooltipContent>
    </Tooltip>
  );
}
