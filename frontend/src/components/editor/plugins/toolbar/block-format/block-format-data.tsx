import {
  CodeIcon,
  Heading1Icon,
  Heading2Icon,
  Heading3Icon,
  ListIcon,
  ListOrderedIcon,
  ListTodoIcon,
  QuoteIcon,
  TextIcon,
} from "lucide-react";

export const blockTypeToBlockName: Record<
  string,
  { labelKey: string; icon: React.ReactNode }
> = {
  paragraph: {
    labelKey: "editor.block.paragraph",
    icon: <TextIcon className="size-4" />,
  },
  h1: {
    labelKey: "editor.block.h1",
    icon: <Heading1Icon className="size-4" />,
  },
  h2: {
    labelKey: "editor.block.h2",
    icon: <Heading2Icon className="size-4" />,
  },
  h3: {
    labelKey: "editor.block.h3",
    icon: <Heading3Icon className="size-4" />,
  },
  number: {
    labelKey: "editor.block.number",
    icon: <ListOrderedIcon className="size-4" />,
  },
  bullet: {
    labelKey: "editor.block.bullet",
    icon: <ListIcon className="size-4" />,
  },
  check: {
    labelKey: "editor.block.check",
    icon: <ListTodoIcon className="size-4" />,
  },
  code: {
    labelKey: "editor.block.code",
    icon: <CodeIcon className="size-4" />,
  },
  quote: {
    labelKey: "editor.block.quote",
    icon: <QuoteIcon className="size-4" />,
  },
};
