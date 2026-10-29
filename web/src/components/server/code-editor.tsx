"use client";

import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { json } from "@codemirror/lang-json";
import { markdown } from "@codemirror/lang-markdown";
import { xml } from "@codemirror/lang-xml";
import { yaml } from "@codemirror/lang-yaml";
import { HighlightStyle, StreamLanguage, bracketMatching, indentOnInput, syntaxHighlighting } from "@codemirror/language";
import { properties } from "@codemirror/legacy-modes/mode/properties";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { toml } from "@codemirror/legacy-modes/mode/toml";
import { highlightSelectionMatches, search, searchKeymap } from "@codemirror/search";
import { EditorState, type Extension } from "@codemirror/state";
import { EditorView, drawSelection, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import { useEffect, useRef } from "react";

const highlight = HighlightStyle.define([
  { tag: [tags.comment, tags.lineComment, tags.blockComment], color: "#71717a", fontStyle: "italic" },
  { tag: [tags.keyword, tags.operatorKeyword, tags.modifier], color: "#c084fc" },
  { tag: [tags.propertyName, tags.definition(tags.propertyName), tags.attributeName], color: "#60a5fa" },
  { tag: [tags.string, tags.special(tags.string), tags.attributeValue], color: "#86efac" },
  { tag: [tags.number, tags.bool, tags.null, tags.atom], color: "#fbbf24" },
  { tag: [tags.tagName, tags.typeName, tags.className], color: "#f472b6" },
  { tag: [tags.heading, tags.strong], color: "#f4f4f5", fontWeight: "600" },
  { tag: [tags.link, tags.url], color: "#60a5fa", textDecoration: "underline" },
  { tag: [tags.meta, tags.punctuation, tags.separator, tags.bracket], color: "#a1a1aa" },
  { tag: tags.invalid, color: "#f87171" },
]);

const theme = EditorView.theme(
  {
    "&": { color: "#e4e4e7", backgroundColor: "transparent" },
    ".cm-content": { caretColor: "#fafafa", padding: "8px 0" },
    ".cm-line": { padding: "0 12px" },
  },
  { dark: true },
);

export type EditorLanguage = "json" | "yaml" | "xml" | "markdown" | "properties" | "toml" | "shell" | "text";

/** Wählt die Sprache für die Syntaxhervorhebung nach dem Dateinamen. */
export function languageFor(name: string): EditorLanguage {
  const lower = name.toLowerCase();
  const extension = lower.includes(".") ? lower.slice(lower.lastIndexOf(".") + 1) : "";
  switch (extension) {
    case "json":
    case "json5":
    case "mcmeta":
      return "json";
    case "yml":
    case "yaml":
      return "yaml";
    case "xml":
    case "html":
    case "svg":
      return "xml";
    case "md":
      return "markdown";
    case "properties":
    case "cfg":
    case "conf":
    case "ini":
    case "lang":
    case "secret":
      return "properties";
    case "toml":
      return "toml";
    case "sh":
    case "bat":
    case "cmd":
      return "shell";
  }
  return "text";
}

const textExtensions = new Set(["txt", "log", "json", "json5", "mcmeta", "yml", "yaml", "xml", "html", "md", "properties", "cfg", "conf", "ini", "lang", "toml", "sh", "bat", "cmd", "csv", "secret", "js", "ts", "css", "sk", "mcfunction", "env", "list", "txt~", "gradle", "json_old", "old", "bak"]);

/** Dateien, die sich als Text bearbeiten lassen (alles andere kann man nur herunterladen). */
export function isEditable(name: string, size: number): boolean {
  if (size > 5 * 1024 * 1024) return false;
  const lower = name.toLowerCase();
  if (!lower.includes(".")) return true; // z. B. "eula" oder "Dockerfile"
  return textExtensions.has(lower.slice(lower.lastIndexOf(".") + 1)) || lower.endsWith(".json.disabled") || lower.endsWith(".yml.disabled");
}

function languageExtension(language: EditorLanguage): Extension {
  switch (language) {
    case "json":
      return json();
    case "yaml":
      return yaml();
    case "xml":
      return xml();
    case "markdown":
      return markdown();
    case "properties":
      return StreamLanguage.define(properties);
    case "toml":
      return StreamLanguage.define(toml);
    case "shell":
      return StreamLanguage.define(shell);
    default:
      return [];
  }
}

export function CodeEditor({ value, language, onChange, onSave, readOnly }: { value: string; language: EditorLanguage; onChange: (value: string) => void; onSave: () => void; readOnly?: boolean }) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const callbacks = useRef({ onChange, onSave });
  callbacks.current = { onChange, onSave };

  useEffect(() => {
    if (!host.current) return;
    const editor = new EditorView({
      parent: host.current,
      state: EditorState.create({
        doc: value,
        extensions: [
          lineNumbers(),
          highlightActiveLine(),
          highlightActiveLineGutter(),
          drawSelection(),
          history(),
          indentOnInput(),
          bracketMatching(),
          search({ top: true }),
          highlightSelectionMatches(),
          syntaxHighlighting(highlight),
          theme,
          languageExtension(language),
          EditorState.readOnly.of(!!readOnly),
          keymap.of([
            { key: "Mod-s", preventDefault: true, run: () => (callbacks.current.onSave(), true) },
            indentWithTab,
            ...searchKeymap,
            ...historyKeymap,
            ...defaultKeymap,
          ]),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) callbacks.current.onChange(update.state.doc.toString());
          }),
        ],
      }),
    });
    view.current = editor;
    return () => {
      editor.destroy();
      view.current = null;
    };
    // Der Editor wird pro Datei neu aufgebaut (die Seite setzt einen key); value ist nur der Startwert
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [language, readOnly]);

  return <div ref={host} className="h-full min-h-[360px] overflow-auto" />;
}
