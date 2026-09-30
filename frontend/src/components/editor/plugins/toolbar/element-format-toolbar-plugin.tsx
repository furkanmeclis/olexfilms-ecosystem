import { useState } from "react";

import { $isLinkNode } from "@lexical/link";
import { $findMatchingParent } from "@lexical/utils";
import {
  $isElementNode,
  $isRangeSelection,
  type BaseSelection,
  type ElementFormatType,
  FORMAT_ELEMENT_COMMAND,
  INDENT_CONTENT_COMMAND,
  OUTDENT_CONTENT_COMMAND,
} from "lexical";

import {
  AlignCenterIcon,
  AlignJustifyIcon,
  AlignLeftIcon,
  AlignRightIcon,
  IndentDecreaseIcon,
  IndentIncreaseIcon,
} from "lucide-react";

import { useToolbarContext } from "@/components/editor/context/toolbar-context";
import { useUpdateToolbarHandler } from "@/components/editor/editor-hooks/use-update-toolbar";
import { getSelectedNode } from "@/components/editor/utils/get-selected-node";
import { Separator } from "@/components/ui/separator";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useLocale } from "@/providers/locale-provider";

const ELEMENT_FORMAT_OPTIONS: {
  [key in Exclude<ElementFormatType, "start" | "end" | "">]: {
    icon: React.ReactNode;
    iconRTL: string;
    labelKey: string;
  };
} = {
  left: {
    icon: <AlignLeftIcon className="size-4" />,
    iconRTL: "left-align",
    labelKey: "editor.align_left",
  },
  center: {
    icon: <AlignCenterIcon className="size-4" />,
    iconRTL: "center-align",
    labelKey: "editor.align_center",
  },
  right: {
    icon: <AlignRightIcon className="size-4" />,
    iconRTL: "right-align",
    labelKey: "editor.align_right",
  },
  justify: {
    icon: <AlignJustifyIcon className="size-4" />,
    iconRTL: "justify-align",
    labelKey: "editor.align_justify",
  },
} as const;

export function ElementFormatToolbarPlugin({
  separator = true,
}: {
  separator?: boolean;
}) {
  const { t } = useLocale();
  const { activeEditor } = useToolbarContext();
  const [elementFormat, setElementFormat] = useState<ElementFormatType>("left");

  const $updateToolbar = (selection: BaseSelection) => {
    if ($isRangeSelection(selection)) {
      const node = getSelectedNode(selection);
      const parent = node.getParent();

      let matchingParent;
      if ($isLinkNode(parent)) {
        // If node is a link, we need to fetch the parent paragraph node to set format
        matchingParent = $findMatchingParent(
          node,
          (parentNode) => $isElementNode(parentNode) && !parentNode.isInline(),
        );
      }
      setElementFormat(
        $isElementNode(matchingParent)
          ? matchingParent.getFormatType()
          : $isElementNode(node)
            ? node.getFormatType()
            : parent?.getFormatType() || "left",
      );
    }
  };

  useUpdateToolbarHandler($updateToolbar);

  const handleValueChange = (value: string) => {
    if (!value) return; // Prevent unselecting current value

    setElementFormat(value as ElementFormatType);

    if (value === "indent") {
      activeEditor.dispatchCommand(INDENT_CONTENT_COMMAND, undefined);
    } else if (value === "outdent") {
      activeEditor.dispatchCommand(OUTDENT_CONTENT_COMMAND, undefined);
    } else {
      activeEditor.dispatchCommand(
        FORMAT_ELEMENT_COMMAND,
        value as ElementFormatType,
      );
    }
  };

  return (
    <>
      <ToggleGroup
        type="single"
        value={elementFormat}
        defaultValue={elementFormat}
        onValueChange={handleValueChange}
      >
        {Object.entries(ELEMENT_FORMAT_OPTIONS).map(([value, option]) => (
          <ToggleGroupItem
            key={value}
            value={value}
            variant={"outline"}
            size="sm"
            aria-label={t(option.labelKey)}
          >
            {option.icon}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {separator && <Separator orientation="vertical" className="h-7!" />}
      <ToggleGroup
        type="single"
        value={elementFormat}
        defaultValue={elementFormat}
        onValueChange={handleValueChange}
      >
        <ToggleGroupItem
          value="outdent"
          aria-label={t("editor.outdent")}
          variant={"outline"}
          size="sm"
        >
          <IndentDecreaseIcon className="size-4" />
        </ToggleGroupItem>

        <ToggleGroupItem
          value="indent"
          variant={"outline"}
          aria-label={t("editor.indent")}
          size="sm"
        >
          <IndentIncreaseIcon className="size-4" />
        </ToggleGroupItem>
      </ToggleGroup>
    </>
  );
}
