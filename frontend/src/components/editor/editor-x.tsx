"use client";

import { CodeHighlightNode, CodeNode } from "@lexical/code";
import {
  AutoFocusExtension,
  ClearEditorExtension,
  DecoratorTextExtension,
  HorizontalRuleExtension,
  SelectionAlwaysOnDisplayExtension,
} from "@lexical/extension";
import { HistoryExtension } from "@lexical/history";
import {
  AutoLinkExtension,
  ClickableLinkExtension,
  LinkExtension,
} from "@lexical/link";
import { CheckListExtension, ListExtension } from "@lexical/list";
import {
  CHECK_LIST,
  ELEMENT_TRANSFORMERS,
  MULTILINE_ELEMENT_TRANSFORMERS,
  TEXT_FORMAT_TRANSFORMERS,
  TEXT_MATCH_TRANSFORMERS,
} from "@lexical/markdown";
import { OverflowNode } from "@lexical/overflow";
import { LexicalExtensionComposer } from "@lexical/react/LexicalExtensionComposer";
import { OnChangePlugin } from "@lexical/react/LexicalOnChangePlugin";
import { TabIndentationPlugin } from "@lexical/react/LexicalTabIndentationPlugin";
import { TablePlugin } from "@lexical/react/LexicalTablePlugin";
import { RichTextExtension } from "@lexical/rich-text";
import { TableCellNode, TableNode, TableRowNode } from "@lexical/table";
import {
  $getRoot,
  $insertNodes,
  configExtension,
  defineExtension,
  type EditorState,
  type LexicalEditor,
} from "lexical";
import {
  $convertFromMarkdownString,
  $convertToMarkdownString,
} from "@lexical/markdown";
import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import { $generateHtmlFromNodes, $generateNodesFromDOM } from "@lexical/html";
import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";

import { ContentEditable } from "@/components/editor/editor-ui/content-editable";
import { DateTimeExtension } from "@/components/editor/extensions/date-time-extension";
import { EmojisExtension } from "@/components/editor/extensions/emojis-extension";
import { MarkdownShortcutsExtension } from "@/components/editor/extensions/markdown-shortcuts-extension";
import { AutocompleteNode } from "@/components/editor/nodes/autocomplete-node";
import { EmojiNode } from "@/components/editor/nodes/emoji-node";
import { LayoutContainerNode } from "@/components/editor/nodes/layout-container-node";
import { LayoutItemNode } from "@/components/editor/nodes/layout-item-node";
import { MentionNode } from "@/components/editor/nodes/mention-node";
import { SpecialTextNode } from "@/components/editor/nodes/special-text-node";
import { ActionsPlugin } from "@/components/editor/plugins/actions/actions-plugin";
import { ClearEditorActionPlugin } from "@/components/editor/plugins/actions/clear-editor-plugin";
import { CounterCharacterPlugin } from "@/components/editor/plugins/actions/counter-character-plugin";
import { EditModeTogglePlugin } from "@/components/editor/plugins/actions/edit-mode-toggle-plugin";
import { MarkdownTogglePlugin } from "@/components/editor/plugins/actions/markdown-toggle-plugin";
import { CodeActionMenuPlugin } from "@/components/editor/plugins/code-action-menu-plugin";
import { CodeHighlightPlugin } from "@/components/editor/plugins/code-highlight-plugin";
import { ComponentPickerMenuPlugin } from "@/components/editor/plugins/component-picker-menu-plugin";
import { ContextMenuPlugin } from "@/components/editor/plugins/context-menu-plugin";
import { DraggableBlockPlugin } from "@/components/editor/plugins/draggable-block-plugin";
import { EmojiPickerPlugin } from "@/components/editor/plugins/emoji-picker-plugin";
import { FloatingLinkEditorPlugin } from "@/components/editor/plugins/floating-link-editor-plugin";
import { FloatingTextFormatToolbarPlugin } from "@/components/editor/plugins/floating-text-format-plugin";
import { LayoutPlugin } from "@/components/editor/plugins/layout-plugin";
import { MentionsPlugin } from "@/components/editor/plugins/mentions-plugin";
import { DateTimePickerPlugin } from "@/components/editor/plugins/picker/date-time-picker-plugin";
import { SpecialTextPlugin } from "@/components/editor/plugins/special-text-plugin";
import { TabFocusPlugin } from "@/components/editor/plugins/tab-focus-plugin";
import { BulletedListPickerPlugin } from "@/components/editor/plugins/picker/bulleted-list-picker-plugin";
import { CheckListPickerPlugin } from "@/components/editor/plugins/picker/check-list-picker-plugin";
import { CodePickerPlugin } from "@/components/editor/plugins/picker/code-picker-plugin";
import { DividerPickerPlugin } from "@/components/editor/plugins/picker/divider-picker-plugin";
import { HeadingPickerPlugin } from "@/components/editor/plugins/picker/heading-picker-plugin";
import { NumberedListPickerPlugin } from "@/components/editor/plugins/picker/numbered-list-picker-plugin";
import { ParagraphPickerPlugin } from "@/components/editor/plugins/picker/paragraph-picker-plugin";
import { QuotePickerPlugin } from "@/components/editor/plugins/picker/quote-picker-plugin";
import { TablePickerPlugin } from "@/components/editor/plugins/picker/table-picker-plugin";
import { BlockFormatDropDown } from "@/components/editor/plugins/toolbar/block-format-toolbar-plugin";
import { FormatBulletedList } from "@/components/editor/plugins/toolbar/block-format/format-bulleted-list";
import { FormatCheckList } from "@/components/editor/plugins/toolbar/block-format/format-check-list";
import { FormatCodeBlock } from "@/components/editor/plugins/toolbar/block-format/format-code-block";
import { FormatHeading } from "@/components/editor/plugins/toolbar/block-format/format-heading";
import { FormatNumberedList } from "@/components/editor/plugins/toolbar/block-format/format-numbered-list";
import { FormatParagraph } from "@/components/editor/plugins/toolbar/block-format/format-paragraph";
import { FormatQuote } from "@/components/editor/plugins/toolbar/block-format/format-quote";
import { BlockInsertPlugin } from "@/components/editor/plugins/toolbar/block-insert-plugin";
import { InsertColumnsLayout } from "@/components/editor/plugins/toolbar/block-insert/insert-columns-layout";
import { InsertHorizontalRule } from "@/components/editor/plugins/toolbar/block-insert/insert-horizontal-rule";
import { InsertTable } from "@/components/editor/plugins/toolbar/block-insert/insert-table";
import { ClearFormattingToolbarPlugin } from "@/components/editor/plugins/toolbar/clear-formatting-toolbar-plugin";
import { CodeLanguageToolbarPlugin } from "@/components/editor/plugins/toolbar/code-language-toolbar-plugin";
import { ElementFormatToolbarPlugin } from "@/components/editor/plugins/toolbar/element-format-toolbar-plugin";
import { FontBackgroundToolbarPlugin } from "@/components/editor/plugins/toolbar/font-background-toolbar-plugin";
import { FontColorToolbarPlugin } from "@/components/editor/plugins/toolbar/font-color-toolbar-plugin";
import { FontFamilyToolbarPlugin } from "@/components/editor/plugins/toolbar/font-family-toolbar-plugin";
import { FontFormatToolbarPlugin } from "@/components/editor/plugins/toolbar/font-format-toolbar-plugin";
import { FontSizeToolbarPlugin } from "@/components/editor/plugins/toolbar/font-size-toolbar-plugin";
import { HistoryToolbarPlugin } from "@/components/editor/plugins/toolbar/history-toolbar-plugin";
import { LinkToolbarPlugin } from "@/components/editor/plugins/toolbar/link-toolbar-plugin";
import { SubSuperToolbarPlugin } from "@/components/editor/plugins/toolbar/subsuper-toolbar-plugin";
import { ToolbarPlugin } from "@/components/editor/plugins/toolbar/toolbar-plugin";
import { editorTheme } from "@/components/editor/themes/editor-theme";
import { EMOJI } from "@/components/editor/transformers/markdown-emoji-transformer";
import { HR } from "@/components/editor/transformers/markdown-hr-transformer";
import { TABLE } from "@/components/editor/transformers/markdown-table-transformer";
import { validateUrl } from "@/components/editor/utils/url";
import { Separator } from "@/components/ui/separator";
import { TooltipProvider } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export const EDITOR_X_MARKDOWN_TRANSFORMERS = [
  TABLE,
  HR,
  EMOJI,
  CHECK_LIST,
  ...ELEMENT_TRANSFORMERS,
  ...MULTILINE_ELEMENT_TRANSFORMERS,
  ...TEXT_FORMAT_TRANSFORMERS,
  ...TEXT_MATCH_TRANSFORMERS,
];

type EditorXProps = {
  /** Markdown / plain text initial content (composer remounts on key change). */
  initialMarkdown?: string;
  /** Emits markdown whenever the document changes. */
  onMarkdownChange?: (markdown: string) => void;
  editable?: boolean;
  placeholder?: string;
  className?: string;
  contentClassName?: string;
  /** Serialized Lexical state; wins over initialHtml / initialMarkdown. */
  initialStateJson?: string | null;
  /** HTML initial content, imported with @lexical/html. */
  initialHtml?: string;
  /** Emits HTML ($generateHtmlFromNodes) and the state JSON on change. */
  onHtmlChange?: (html: string, stateJson: string) => void;
  /** Extra toolbar items, rendered inside the editor context. */
  toolbarExtra?: ReactNode;
};

function HtmlChangePlugin({
  onHtmlChange,
}: {
  onHtmlChange?: (html: string, stateJson: string) => void;
}) {
  const [editor] = useLexicalComposerContext();
  const skipFirst = useRef(true);

  return (
    <OnChangePlugin
      ignoreSelectionChange
      onChange={(editorState: EditorState) => {
        if (!onHtmlChange) return;
        if (skipFirst.current) {
          skipFirst.current = false;
          return;
        }
        editorState.read(() => {
          const html = $generateHtmlFromNodes(editor, null);
          onHtmlChange(html, JSON.stringify(editorState.toJSON()));
        });
      }}
    />
  );
}

function $importHtml(editor: LexicalEditor, html: string) {
  const dom = new DOMParser().parseFromString(html, "text/html");
  const nodes = $generateNodesFromDOM(editor, dom);
  $getRoot().select();
  $insertNodes(nodes);
}

function EditablePlugin({ editable }: { editable: boolean }) {
  const [editor] = useLexicalComposerContext();
  useEffect(() => {
    editor.setEditable(editable);
  }, [editor, editable]);
  return null;
}

function MarkdownChangePlugin({
  onMarkdownChange,
}: {
  onMarkdownChange?: (markdown: string) => void;
}) {
  const skipFirst = useRef(true);

  return (
    <OnChangePlugin
      ignoreSelectionChange
      onChange={(editorState: EditorState) => {
        if (!onMarkdownChange) return;
        if (skipFirst.current) {
          skipFirst.current = false;
          return;
        }
        editorState.read(() => {
          const markdown = $convertToMarkdownString(
            EDITOR_X_MARKDOWN_TRANSFORMERS,
            undefined,
            true,
          );
          onMarkdownChange(markdown);
        });
      }}
    />
  );
}

/**
 * Shadcn Editor X — Lexical rich text editor for knowledge document bodies.
 * Persists as markdown string (API plain-text body).
 */
export function EditorX({
  initialMarkdown = "",
  onMarkdownChange,
  editable = true,
  placeholder,
  className,
  contentClassName,
  initialStateJson,
  initialHtml,
  onHtmlChange,
  toolbarExtra,
}: EditorXProps) {
  const { t } = useLocale();
  const resolvedPlaceholder = placeholder ?? t("editor.placeholder");
  const [floatingAnchorElem, setFloatingAnchorElem] =
    useState<HTMLDivElement | null>(null);
  const [isLinkEditMode, setIsLinkEditMode] = useState(false);

  const onRef = (node: HTMLDivElement) => {
    if (node !== null) setFloatingAnchorElem(node);
  };

  const AppExtension = useMemo(
    () =>
      defineExtension({
        dependencies: [
          RichTextExtension,
          AutoFocusExtension,
          SelectionAlwaysOnDisplayExtension,
          HistoryExtension,
          configExtension(LinkExtension, {
            validateUrl,
            attributes: { rel: "noopener noreferrer", target: "_blank" },
          }),
          AutoLinkExtension,
          ClickableLinkExtension,
          configExtension(MarkdownShortcutsExtension, {
            transformers: EDITOR_X_MARKDOWN_TRANSFORMERS,
          }),
          ClearEditorExtension,
          EmojisExtension,
          DecoratorTextExtension,
          configExtension(ListExtension, { shouldPreserveNumbering: false }),
          CheckListExtension,
          HorizontalRuleExtension,
          DateTimeExtension,
        ],
        name: "@shadcn-editor",
        namespace: "Knowledge",
        nodes: [
          OverflowNode,
          EmojiNode,
          MentionNode,
          AutocompleteNode,
          SpecialTextNode,
          CodeNode,
          CodeHighlightNode,
          TableNode,
          TableCellNode,
          TableRowNode,
          LayoutContainerNode,
          LayoutItemNode,
        ],
        $initialEditorState: initialStateJson
          ? initialStateJson
          : (editor: LexicalEditor) => {
              const root = $getRoot();
              if (root.getFirstChild() !== null) return;
              if (initialHtml) {
                $importHtml(editor, initialHtml);
                return;
              }
              $convertFromMarkdownString(
                initialMarkdown,
                EDITOR_X_MARKDOWN_TRANSFORMERS,
                undefined,
                true,
              );
            },
        theme: editorTheme,
      }),
    // Remount via `key` when initialMarkdown identity changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  return (
    <div
      className={cn(
        "bg-background w-full max-w-full min-w-0 overflow-hidden rounded-lg border shadow-xs",
        className,
      )}
    >
      <LexicalExtensionComposer extension={AppExtension} contentEditable={null}>
        <TooltipProvider>
          <div className="relative">
            <ToolbarPlugin>
              {({ blockType }) => (
                <div className="vertical-align-middle sticky top-0 z-10 flex min-w-0 items-center gap-2 overflow-x-auto border-b p-1">
                  <HistoryToolbarPlugin />
                  <Separator orientation="vertical" className="h-7!" />
                  <BlockFormatDropDown>
                    <FormatParagraph />
                    <FormatHeading levels={["h1", "h2", "h3"]} />
                    <FormatNumberedList />
                    <FormatBulletedList />
                    <FormatCheckList />
                    <FormatCodeBlock />
                    <FormatQuote />
                  </BlockFormatDropDown>
                  {blockType === "code" ? (
                    <CodeLanguageToolbarPlugin />
                  ) : (
                    <>
                      <FontFamilyToolbarPlugin />
                      <Separator orientation="vertical" className="h-7!" />
                      <FontSizeToolbarPlugin />
                      <FontFormatToolbarPlugin />
                      <SubSuperToolbarPlugin />
                      <LinkToolbarPlugin
                        setIsLinkEditMode={setIsLinkEditMode}
                      />
                      <ClearFormattingToolbarPlugin />
                      <FontColorToolbarPlugin />
                      <FontBackgroundToolbarPlugin />
                      <ElementFormatToolbarPlugin />
                      <BlockInsertPlugin>
                        <InsertHorizontalRule />
                        <InsertTable />
                        <InsertColumnsLayout />
                      </BlockInsertPlugin>
                      {toolbarExtra}
                    </>
                  )}
                </div>
              )}
            </ToolbarPlugin>

            <div className="relative">
              <div ref={onRef}>
                <ContentEditable
                  placeholder={resolvedPlaceholder}
                  className={cn("min-h-72", contentClassName)}
                />
              </div>
              <ComponentPickerMenuPlugin
                baseOptions={[
                  DateTimePickerPlugin({ title: t("editor.picker.date") }),
                  HeadingPickerPlugin({
                    n: 1,
                    title: t("editor.picker.heading_1"),
                  }),
                  HeadingPickerPlugin({
                    n: 2,
                    title: t("editor.picker.heading_2"),
                  }),
                  HeadingPickerPlugin({
                    n: 3,
                    title: t("editor.picker.heading_3"),
                  }),
                  ParagraphPickerPlugin({
                    title: t("editor.picker.paragraph"),
                  }),
                  QuotePickerPlugin({ title: t("editor.picker.quote") }),
                  CodePickerPlugin({ title: t("editor.picker.code") }),
                  NumberedListPickerPlugin({
                    title: t("editor.picker.numbered_list"),
                  }),
                  BulletedListPickerPlugin({
                    title: t("editor.picker.bulleted_list"),
                  }),
                  CheckListPickerPlugin({
                    title: t("editor.picker.check_list"),
                  }),
                  DividerPickerPlugin({ title: t("editor.picker.divider") }),
                  TablePickerPlugin({
                    title: t("editor.picker.table"),
                    modalTitle: t("editor.insert.table_title"),
                  }),
                ]}
              />
              <EmojiPickerPlugin />
              <MentionsPlugin />
              <ContextMenuPlugin />
              <SpecialTextPlugin />
              <TabFocusPlugin />
              <TabIndentationPlugin />
              <CodeHighlightPlugin />
              <TablePlugin />
              <LayoutPlugin />
              <DraggableBlockPlugin
                anchorElem={floatingAnchorElem}
                baseOptions={[]}
              />
              <FloatingTextFormatToolbarPlugin
                anchorElem={floatingAnchorElem}
                setIsLinkEditMode={setIsLinkEditMode}
              />
              <FloatingLinkEditorPlugin
                anchorElem={floatingAnchorElem}
                isLinkEditMode={isLinkEditMode}
                setIsLinkEditMode={setIsLinkEditMode}
              />
              <CodeActionMenuPlugin anchorElem={floatingAnchorElem} />
            </div>

            <ActionsPlugin>
              <div className="clear-both flex items-center justify-between gap-2 overflow-auto border-t p-1">
                <div className="text-muted-foreground flex flex-1 justify-start text-xs">
                  <CounterCharacterPlugin charset="UTF-16" />
                </div>
                <div className="flex flex-1 justify-end">
                  <MarkdownTogglePlugin
                    shouldPreserveNewLinesInMarkdown
                    transformers={EDITOR_X_MARKDOWN_TRANSFORMERS}
                  />
                  <EditModeTogglePlugin />
                  <ClearEditorActionPlugin />
                </div>
              </div>
            </ActionsPlugin>
          </div>

          <EditablePlugin editable={editable} />
          <MarkdownChangePlugin onMarkdownChange={onMarkdownChange} />
          <HtmlChangePlugin onHtmlChange={onHtmlChange} />
        </TooltipProvider>
      </LexicalExtensionComposer>
    </div>
  );
}
