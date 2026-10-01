"use client";

import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import { $getSelection, $isRangeSelection } from "lexical";
import { Braces } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  groupVariables,
  placeholder,
} from "@/features/document-templates/lib/variables";
import type { DocumentVariable } from "@/features/document-templates/services/document-templates.service";
import { useLocale } from "@/providers/locale-provider";

type VariableMenuProps = {
  variables: readonly DocumentVariable[];
  onInsert: (key: string) => void;
  disabled?: boolean;
};

/** Dropdown of the kind's variables; inserts `{{key}}`. */
export function VariableMenu({
  variables,
  onInsert,
  disabled,
}: VariableMenuProps) {
  const { t, locale } = useLocale();
  const groups = groupVariables(variables);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={disabled}>
          <Braces className="size-4" />
          {t("documents.editor.insert_variable")}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="max-h-96 w-72" align="start">
        {groups.map((g, i) => (
          <DropdownMenuGroup key={g.group}>
            {i > 0 ? <DropdownMenuSeparator /> : null}
            <DropdownMenuLabel>
              {t(`documents.variable_groups.${g.group}`)}
            </DropdownMenuLabel>
            {g.items.map((v) => (
              <DropdownMenuItem key={v.key} onSelect={() => onInsert(v.key)}>
                <span className="flex-1">
                  {locale === "tr" ? v.label_tr : v.label_en}
                </span>
                <code className="text-muted-foreground text-xs">
                  {placeholder(v.key)}
                </code>
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** VariableMenu wired to the Lexical selection (rendered in EditorX toolbar). */
export function LexicalVariableMenu({
  variables,
}: {
  variables: readonly DocumentVariable[];
}) {
  const [editor] = useLexicalComposerContext();
  return (
    <VariableMenu
      variables={variables}
      onInsert={(key) =>
        editor.update(() => {
          const selection = $getSelection();
          if ($isRangeSelection(selection)) {
            selection.insertText(placeholder(key));
          }
        })
      }
    />
  );
}
