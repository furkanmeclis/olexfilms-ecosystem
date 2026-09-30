import { PlusIcon } from "lucide-react";

import { useEditorModal } from "@/components/editor/editor-hooks/use-modal";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useLocale } from "@/providers/locale-provider";

export function BlockInsertPlugin({ children }: { children: React.ReactNode }) {
  const { t } = useLocale();
  const [modal] = useEditorModal();

  return (
    <>
      {modal}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm" className="gap-1 px-2">
            <PlusIcon className="size-4" />
            <span className="text-sm">{t("editor.insert")}</span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-fit">{children}</DropdownMenuContent>
      </DropdownMenu>
    </>
  );
}
