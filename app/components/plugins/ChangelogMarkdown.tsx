import ReactMarkdown, { type Components } from "react-markdown";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";

import { cn } from "@/libraries/utils";

const schema = {
  ...defaultSchema,
  tagNames: [
    "h2",
    "h3",
    "p",
    "ul",
    "ol",
    "li",
    "code",
    "pre",
    "a",
    "strong",
    "em",
  ],
  attributes: {
    a: ["href", "title"],
    code: [],
    pre: [],
  },
};

/* eslint-disable @typescript-eslint/no-unused-vars */
// react-markdown passes a `node` prop that must not be forwarded to the DOM.
// We destructure it off and spread the remaining props. The `_node` bindings
// are intentionally unused.
const components: Components = {
  a: ({ node: _node, ...props }) => (
    <a
      {...props}
      className="underline underline-offset-2 hover:text-foreground"
      rel="noopener noreferrer"
      target="_blank"
    />
  ),
  code: ({ node: _node, ...props }) => (
    <code
      {...props}
      className="rounded bg-muted px-1 py-0.5 font-mono text-xs"
    />
  ),
  h2: ({ node: _node, ...props }) => (
    <h2
      {...props}
      className="mt-4 text-base font-semibold text-foreground first:mt-0"
    />
  ),
  h3: ({ node: _node, ...props }) => (
    <h3 {...props} className="mt-3 text-sm font-semibold text-foreground" />
  ),
  li: ({ node: _node, ...props }) => (
    <li {...props} className="leading-relaxed" />
  ),
  ol: ({ node: _node, ...props }) => (
    <ol {...props} className="list-decimal space-y-1 pl-5" />
  ),
  p: ({ node: _node, ...props }) => (
    <p {...props} className="leading-relaxed" />
  ),
  pre: ({ node: _node, ...props }) => (
    <pre
      {...props}
      className="overflow-x-auto rounded bg-muted p-3 font-mono text-xs"
    />
  ),
  strong: ({ node: _node, ...props }) => (
    <strong {...props} className="font-semibold text-foreground" />
  ),
  ul: ({ node: _node, ...props }) => (
    <ul {...props} className="list-disc space-y-1 pl-5" />
  ),
};
/* eslint-enable @typescript-eslint/no-unused-vars */

export interface ChangelogMarkdownProps {
  children: string;
  className?: string;
}

export const ChangelogMarkdown = ({
  children,
  className,
}: ChangelogMarkdownProps) => (
  <div className={cn("space-y-2 text-sm text-muted-foreground", className)}>
    <ReactMarkdown
      components={components}
      rehypePlugins={[[rehypeSanitize, schema]]}
    >
      {children}
    </ReactMarkdown>
  </div>
);
