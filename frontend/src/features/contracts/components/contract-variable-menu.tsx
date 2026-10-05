"use client";

import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
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
  $insertVariable,
  variableToken,
} from "@/features/contracts/lib/variable-node";
import type { ContractTemplateVariable } from "@/features/contracts/services/contract-templates.service";
import { useLocale } from "@/providers/locale-provider";

/** Variables grouped in API order. */
export function groupContractVariables(
  variables: readonly ContractTemplateVariable[],
): { group: string; items: ContractTemplateVariable[] }[] {
  const groups = new Map<string, ContractTemplateVariable[]>();
  for (const v of variables) {
    const list = groups.get(v.group) ?? [];
    list.push(v);
    groups.set(v.group, list);
  }
  return [...groups.entries()].map(([group, items]) => ({ group, items }));
}

type ContractVariableMenuProps = {
  variables: readonly ContractTemplateVariable[];
  onInsert: (key: string) => void;
  disabled?: boolean;
};

/** "Insert variable" dropdown of the allowed contract variables. */
export function ContractVariableMenu({
  variables,
  onInsert,
  disabled,
}: ContractVariableMenuProps) {
  const { t, locale } = useLocale();
  const groups = groupContractVariables(variables);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled || variables.length === 0}
        >
          <Braces className="size-4" />
          {t("contract_templates.editor.insert_variable")}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="max-h-96 w-72" align="start">
        {groups.map((g, i) => (
          <DropdownMenuGroup key={g.group}>
            {i > 0 ? <DropdownMenuSeparator /> : null}
            <DropdownMenuLabel>
              {t(`contract_templates.variable_groups.${g.group}`)}
            </DropdownMenuLabel>
            {g.items.map((v) => (
              <DropdownMenuItem key={v.key} onSelect={() => onInsert(v.key)}>
                <span className="flex-1">
                  {locale === "tr" ? v.label_tr : v.label_en}
                </span>
                <code className="text-muted-foreground text-xs">
                  {variableToken(v.key)}
                </code>
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Toolbar menu wired to the Lexical selection; inserts a variable token. */
export function LexicalContractVariableMenu({
  variables,
  disabled,
}: {
  variables: readonly ContractTemplateVariable[];
  disabled?: boolean;
}) {
  const [editor] = useLexicalComposerContext();
  return (
    <ContractVariableMenu
      variables={variables}
      disabled={disabled}
      onInsert={(key) =>
        editor.update(() => {
          $insertVariable(key);
        })
      }
    />
  );
}
