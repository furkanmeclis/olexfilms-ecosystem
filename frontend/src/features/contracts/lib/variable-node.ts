import {
  $applyNodeReplacement,
  $createParagraphNode,
  $getRoot,
  $getSelection,
  $isElementNode,
  $isRangeSelection,
  defineExtension,
  TextNode,
  type EditorConfig,
  type LexicalNode,
  type SerializedTextNode,
} from "lexical";

/** Same pattern as the backend (msgtemplate.placeholderRE). */
const VARIABLE_RE = /\{\{\s*\.?([a-zA-Z0-9_]+)\s*\}\}/;

/** `{{key}}` placeholder stored in HTML and Lexical state. */
export function variableToken(key: string) {
  return `{{${key}}}`;
}

const TOKEN_CLASS =
  "contract-variable rounded-sm bg-primary/10 px-1 font-mono text-primary";

/**
 * Contract variable token: an immutable text entity whose text is the
 * `{{key}}` placeholder, so the HTML export keeps the placeholder.
 */
export class VariableNode extends TextNode {
  static getType(): string {
    return "contract-variable";
  }

  static clone(node: VariableNode): VariableNode {
    return new VariableNode(node.__text, node.__key);
  }

  static importJSON(serialized: SerializedTextNode): VariableNode {
    return $createVariableNode("").updateFromJSON(serialized);
  }

  createDOM(config: EditorConfig): HTMLElement {
    const dom = super.createDOM(config);
    dom.className = TOKEN_CLASS;
    dom.dataset.contractVariable = variableKey(this.__text) ?? "";
    dom.spellcheck = false;
    return dom;
  }

  canInsertTextBefore(): boolean {
    return false;
  }

  canInsertTextAfter(): boolean {
    return false;
  }

  isTextEntity(): true {
    return true;
  }
}

/** Variable key of a `{{key}}` token text, lower-cased like the backend. */
export function variableKey(text: string): string | null {
  const match = VARIABLE_RE.exec(text);
  return match ? match[1].toLowerCase() : null;
}

export function $createVariableNode(key: string): VariableNode {
  const node = new VariableNode(key ? variableToken(key) : "");
  node.setMode("token");
  return $applyNodeReplacement(node);
}

export function $isVariableNode(
  node: LexicalNode | null | undefined,
): node is VariableNode {
  return node instanceof VariableNode;
}

/** Inserts a variable token at the selection (or at the document end). */
export function $insertVariable(key: string): VariableNode {
  const node = $createVariableNode(key);
  const selection = $getSelection();
  if ($isRangeSelection(selection)) {
    selection.insertNodes([node]);
  } else {
    const root = $getRoot();
    const last = root.getLastChild();
    if ($isElementNode(last)) {
      last.append(node);
    } else {
      root.append($createParagraphNode().append(node));
    }
  }
  return node;
}

/**
 * Turns typed or imported `{{key}}` text (HTML without Lexical state) into
 * tokens. Tokens are immutable, so only plain text needs a transform.
 */
function $tokenizeVariables(node: TextNode) {
  if (!node.isSimpleText()) return;
  const match = VARIABLE_RE.exec(node.getTextContent());
  if (!match) return;
  const start = match.index;
  const end = start + match[0].length;
  const target =
    start === 0 ? node.splitText(end)[0] : node.splitText(start, end)[1];
  if (!target) return;
  const token = $createVariableNode("");
  token.setTextContent(target.getTextContent());
  token.setFormat(target.getFormat());
  target.replace(token);
}

/** Registers VariableNode and the `{{key}}` text transform. */
export const ContractVariablesExtension = defineExtension({
  name: "@olex/contract-variables",
  nodes: () => [VariableNode],
  register(editor) {
    return editor.registerNodeTransform(TextNode, $tokenizeVariables);
  },
});
