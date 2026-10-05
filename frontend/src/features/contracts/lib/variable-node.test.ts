// @vitest-environment jsdom
import { buildEditorFromExtensions } from "@lexical/extension";
import { $generateHtmlFromNodes } from "@lexical/html";
import { RichTextExtension } from "@lexical/rich-text";
import {
  $createParagraphNode,
  $createTextNode,
  $getRoot,
  defineExtension,
  type LexicalEditor,
} from "lexical";
import { afterEach, describe, expect, it } from "vitest";

import {
  $insertVariable,
  $isVariableNode,
  ContractVariablesExtension,
  variableKey,
} from "@/features/contracts/lib/variable-node";

let editor: (LexicalEditor & { dispose: () => void }) | null = null;

function build() {
  editor = buildEditorFromExtensions(
    defineExtension({
      name: "[test]",
      dependencies: [RichTextExtension, ContractVariablesExtension],
      $initialEditorState: () => {
        $getRoot().append(
          $createParagraphNode().append($createTextNode("Dear ")),
        );
      },
    }),
  );
  return editor;
}

afterEach(() => {
  editor?.dispose();
  editor = null;
});

function html(e: LexicalEditor) {
  return e.getEditorState().read(() => $generateHtmlFromNodes(e, null));
}

type JsonNode = { type: string; text?: string; children?: JsonNode[] };

function flatten(node: JsonNode): JsonNode[] {
  return [node, ...(node.children ?? []).flatMap(flatten)];
}

describe("contract variable token (TEC-290)", () => {
  it("inserts {{customer_name}} as a token into the Lexical state and HTML", () => {
    const e = build();
    e.update(
      () => {
        $getRoot().selectEnd();
        $insertVariable("customer_name");
      },
      { discrete: true },
    );

    const state = e.getEditorState().toJSON() as unknown as {
      root: JsonNode;
    };
    const tokens = flatten(state.root).filter(
      (n) => n.type === "contract-variable",
    );
    expect(tokens).toHaveLength(1);
    expect(tokens[0].text).toBe("{{customer_name}}");

    e.getEditorState().read(() => {
      const last = $getRoot().getLastDescendant();
      expect($isVariableNode(last)).toBe(true);
      expect(last?.getTextContent()).toBe("{{customer_name}}");
    });

    const out = html(e);
    expect(out).toContain("Dear ");
    expect(out).toContain("{{customer_name}}");
  });

  it("restores the token from saved Lexical JSON", () => {
    const e = build();
    e.update(
      () => {
        $getRoot().selectEnd();
        $insertVariable("plate");
      },
      { discrete: true },
    );
    const saved = JSON.stringify(e.getEditorState().toJSON());

    const other = build();
    other.setEditorState(other.parseEditorState(saved));
    expect(html(other)).toContain("{{plate}}");
    other.getEditorState().read(() => {
      expect(
        $getRoot()
          .getAllTextNodes()
          .some((n) => $isVariableNode(n)),
      ).toBe(true);
    });
  });

  it("turns typed {{key}} text into a token", () => {
    const e = build();
    e.update(
      () => {
        $getRoot()
          .getFirstChildOrThrow<ReturnType<typeof $createParagraphNode>>()
          .append($createTextNode("plate: {{ plate }} ok"));
      },
      { discrete: true },
    );
    e.getEditorState().read(() => {
      const tokens = $getRoot().getAllTextNodes().filter($isVariableNode);
      expect(tokens.map((n) => n.getTextContent())).toEqual(["{{ plate }}"]);
    });
  });

  it("reads the variable key of a token", () => {
    expect(variableKey("{{Customer_Name}}")).toBe("customer_name");
    expect(variableKey("plain")).toBeNull();
  });
});
