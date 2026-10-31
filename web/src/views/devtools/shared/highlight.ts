/**
 * 极简语法高亮：把代码切成带类别的片段，由组件用 <span> 渲染（不使用 v-html）。
 * 只识别注释、字符串、数字、关键字、标签和 JSON 键，足够阅读生成的代码和示例。
 */

export type Language = "go" | "typescript" | "vue" | "json";

export type TokenKind =
  "plain" | "comment" | "string" | "number" | "keyword" | "tag" | "key";

export type Token = { text: string; kind: TokenKind };

const KEYWORDS: Record<Language, ReadonlySet<string>> = {
  go: new Set(
    "break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var nil true false any error string bool int int16 int64 float64 byte".split(
      " "
    )
  ),
  typescript: new Set(
    "as async await break case catch class const continue default delete do else export extends false finally for from function if import in instanceof let new null of return switch this throw true try type typeof undefined var void while string number boolean unknown".split(
      " "
    )
  ),
  vue: new Set(
    "as async await const else export false from function if import let new null return true type undefined void string number boolean".split(
      " "
    )
  ),
  json: new Set(["true", "false", "null"])
};

type State = { blockEnd: string | null; inBacktick: boolean };

const STRING_RE = /^("(?:[^"\\]|\\.)*"?|'(?:[^'\\]|\\.)*'?)/;
const NUMBER_RE = /^-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b/;
const IDENT_RE = /^[A-Za-z_$][\w$]*/;
const TAG_RE = /^<\/?[A-Za-z][\w-]*|^\/?>/;

function push(out: Token[], text: string, kind: TokenKind): void {
  if (!text) return;
  const last = out[out.length - 1];
  if (last && last.kind === kind) {
    last.text += text;
    return;
  }
  out.push({ text, kind });
}

/** 处理跨行的块注释或反引号字符串，返回已消费的长度 */
function continueState(line: string, state: State, out: Token[]): number {
  if (state.blockEnd) {
    const end = line.indexOf(state.blockEnd);
    if (end < 0) {
      push(out, line, "comment");
      return line.length;
    }
    const stop = end + state.blockEnd.length;
    push(out, line.slice(0, stop), "comment");
    state.blockEnd = null;
    return stop;
  }
  if (state.inBacktick) {
    const end = line.indexOf("`");
    if (end < 0) {
      push(out, line, "string");
      return line.length;
    }
    push(out, line.slice(0, end + 1), "string");
    state.inBacktick = false;
    return end + 1;
  }
  return 0;
}

function scanSpecial(
  rest: string,
  lang: Language,
  state: State,
  out: Token[]
): number {
  if (lang !== "json" && rest.startsWith("//")) {
    push(out, rest, "comment");
    return rest.length;
  }
  for (const [open, close] of [
    ["/*", "*/"],
    ["<!--", "-->"]
  ]) {
    if (!rest.startsWith(open)) continue;
    const end = rest.indexOf(close, open.length);
    if (end < 0) {
      state.blockEnd = close;
      push(out, rest, "comment");
      return rest.length;
    }
    push(out, rest.slice(0, end + close.length), "comment");
    return end + close.length;
  }
  if (rest.startsWith("`") && lang !== "json") {
    const end = rest.indexOf("`", 1);
    if (end < 0) {
      state.inBacktick = true;
      push(out, rest, "string");
      return rest.length;
    }
    push(out, rest.slice(0, end + 1), "string");
    return end + 1;
  }
  return 0;
}

function scanToken(rest: string, lang: Language, out: Token[]): number {
  const str = STRING_RE.exec(rest);
  if (str) {
    const isKey = lang === "json" && /^\s*:/.test(rest.slice(str[0].length));
    push(out, str[0], isKey ? "key" : "string");
    return str[0].length;
  }
  const num = NUMBER_RE.exec(rest);
  if (num) {
    push(out, num[0], "number");
    return num[0].length;
  }
  const ident = IDENT_RE.exec(rest);
  if (ident) {
    push(out, ident[0], KEYWORDS[lang].has(ident[0]) ? "keyword" : "plain");
    return ident[0].length;
  }
  if (lang === "vue") {
    const tag = TAG_RE.exec(rest);
    if (tag) {
      push(out, tag[0], "tag");
      return tag[0].length;
    }
  }
  push(out, rest[0], "plain");
  return 1;
}

function tokenizeLine(line: string, lang: Language, state: State): Token[] {
  const out: Token[] = [];
  let i = continueState(line, state, out);
  while (i < line.length) {
    const rest = line.slice(i);
    const used = scanSpecial(rest, lang, state, out);
    i += used > 0 ? used : scanToken(rest, lang, out);
  }
  return out;
}

/** 按行切分并高亮 */
export function highlight(code: string, lang: Language): Token[][] {
  const state: State = { blockEnd: null, inBacktick: false };
  return code
    .replace(/\n$/, "")
    .split("\n")
    .map(line => tokenizeLine(line, lang, state));
}
