// A light reading of Gherkin, enough to build the Test Explorer tree:
// feature, background, scenarios, outlines with their example rows, tags
// and step lines. apic's own parser (godog's) runs the features; this
// only needs to know where things are. Pure: unit-tested under node.

export interface Step {
  keyword: string;
  text: string;
  /** 1-based. */
  line: number;
}

export interface ExampleRow {
  line: number;
  cells: string[];
}

export interface Examples {
  name: string;
  line: number;
  tags: string[];
  header: string[];
  rows: ExampleRow[];
}

export interface Scenario {
  keyword: "Scenario" | "Scenario Outline";
  name: string;
  line: number;
  /** The scenario's own tags and the feature's. */
  tags: string[];
  steps: Step[];
  examples: Examples[];
}

export interface Feature {
  name: string;
  line: number;
  tags: string[];
  background?: { line: number; steps: Step[] };
  scenarios: Scenario[];
}

const keywordRe = /^\s*(Feature|Background|Scenario Outline|Scenario Template|Scenario|Example|Examples|Scenarios|Rule):\s*(.*)$/;
const stepRe = /^\s*(Given|When|Then|And|But|\*)\s+(.*?)\s*$/;
const tagRe = /^\s*@\S/;
const docStringRe = /^\s*("""|```)/;

/** Where parseFeature is: the feature so far, the section a line belongs to and the tags waiting for a keyword. */
interface ParseState {
  feature?: Feature;
  scenario?: Scenario;
  examples?: Examples;
  background?: { line: number; steps: Step[] };
  tags: string[];
}

/** Parses a feature file; undefined when it has no `Feature:` line. */
export function parseFeature(text: string): Feature | undefined {
  const state: ParseState = { tags: [] };
  let inDocString: string | undefined;
  const lines = text.split(/\r?\n/);
  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i];
    const line = i + 1;
    const doc = docStringRe.exec(raw);
    if (doc) {
      inDocString = inDocString === doc[1] ? undefined : (inDocString ?? doc[1]);
      continue;
    }
    if (inDocString) {
      continue;
    }
    const trimmed = raw.trim();
    if (trimmed === "" || trimmed.startsWith("#")) {
      continue;
    }
    if (tagRe.test(raw)) {
      state.tags.push(...trimmed.split(/\s+/).filter((t) => t.startsWith("@")));
      continue;
    }
    const kw = keywordRe.exec(raw);
    if (kw) {
      applyKeyword(state, kw[1], kw[2].trim(), line);
      state.tags = [];
      continue;
    }
    if (trimmed.startsWith("|")) {
      if (state.examples) {
        addTableRow(state.examples, trimmed, line);
      }
      continue; // a step's data table otherwise
    }
    const st = stepRe.exec(raw);
    if (st) {
      addStep(state, { keyword: st[1], text: st[2], line });
    }
    // Anything else is a description line.
  }
  return state.feature;
}

/** Opens the section a keyword line starts, taking the tags read before it. */
function applyKeyword(state: ParseState, keyword: string, name: string, line: number): void {
  switch (keyword) {
    case "Feature":
      state.feature = { name, line, tags: state.tags, scenarios: [] };
      break;
    case "Background":
      state.background = { line, steps: [] };
      if (state.feature) {
        state.feature.background = state.background;
      }
      state.scenario = undefined;
      state.examples = undefined;
      break;
    case "Scenario":
    case "Example":
    case "Scenario Outline":
    case "Scenario Template":
      state.scenario = newScenario(state, keyword, name, line);
      state.feature?.scenarios.push(state.scenario);
      state.background = undefined;
      state.examples = undefined;
      break;
    case "Examples":
    case "Scenarios":
      state.examples = { name, line, tags: state.tags, header: [], rows: [] };
      state.scenario?.examples.push(state.examples);
      break;
    case "Rule":
      state.scenario = undefined;
      state.examples = undefined;
      state.background = undefined;
      break;
  }
}

/** A scenario or outline under any of its keywords, with the feature's tags ahead of its own. */
function newScenario(state: ParseState, keyword: string, name: string, line: number): Scenario {
  const plain = keyword === "Scenario" || keyword === "Example";
  return { keyword: plain ? "Scenario" : "Scenario Outline", name, line, tags: [...(state.feature?.tags ?? []), ...state.tags], steps: [], examples: [] };
}

/** An examples table's line: the first is the header, the rest are rows. */
function addTableRow(examples: Examples, trimmed: string, line: number): void {
  const cells = trimmed
    .slice(1, trimmed.endsWith("|") ? -1 : undefined)
    .split("|")
    .map((c) => c.trim());
  if (examples.header.length === 0) {
    examples.header = cells;
  } else {
    examples.rows.push({ line, cells });
  }
}

/** A step belongs to the open scenario, else the background; one under an examples table is ignored. */
function addStep(state: ParseState, step: Step): void {
  if (state.examples) {
    return;
  }
  if (state.scenario) {
    state.scenario.steps.push(step);
  } else if (state.background) {
    state.background.steps.push(step);
  }
}

/** The label the tree shows for an example row: its cells, in order. */
export function rowLabel(row: ExampleRow, header: string[]): string {
  return row.cells.map((c, i) => (header[i] ? `${header[i]} = ${c}` : c)).join(", ");
}

/**
 * `test.paths` from an apic.yaml's text, or undefined when it is not set.
 * Only the shapes the schema allows are read: a block list or a flow
 * list of strings under `test:`.
 */
export function testPathsFromYaml(text: string): string[] | undefined {
  const lines = text.split(/\r?\n/);
  let inTest = false;
  let testIndent = 0;
  let inPaths = false;
  let pathsIndent = 0;
  const paths: string[] = [];
  const unquote = (s: string) => s.trim().replace(/^["']|["']$/g, "");
  for (const raw of lines) {
    const line = raw.replace(/\s+#.*$/, "").trimEnd();
    if (line.trim() === "" || line.trim().startsWith("#")) {
      continue;
    }
    const indent = line.length - line.trimStart().length;
    if (!inTest) {
      if (/^test:\s*$/.test(line)) {
        inTest = true;
        testIndent = indent;
      }
      continue;
    }
    if (indent <= testIndent && !inPaths) {
      return undefined; // test: ended without paths
    }
    if (!inPaths) {
      const m = /^\s*paths:\s*(.*)$/.exec(line);
      if (m && indent > testIndent) {
        if (m[1].startsWith("[")) {
          return m[1]
            .replace(/^\[|\]$/g, "")
            .split(",")
            .map(unquote)
            .filter(Boolean);
        }
        if (m[1].trim()) {
          return [unquote(m[1])];
        }
        inPaths = true;
        pathsIndent = indent;
      }
      continue;
    }
    const item = /^\s*-\s*(.*)$/.exec(line);
    if (item && indent > pathsIndent) {
      paths.push(unquote(item[1]));
    } else {
      break;
    }
  }
  return inPaths ? paths : undefined;
}

/**
 * The regular expression a `# @step` phrase matches, as apic builds it:
 * `{name}` matches a quoted string or one word, `"{name}"` the inside of
 * a quoted string.
 */
export function stepRegex(phrase: string): RegExp {
  const paramRe = /"\{[A-Za-z_][\w.-]*\}"|\{[A-Za-z_][\w.-]*\}/g;
  let out = "^";
  let last = 0;
  for (const m of phrase.trim().matchAll(paramRe)) {
    out += escapeRegExp(phrase.trim().slice(last, m.index));
    out += m[0].startsWith('"') ? '"([^"]*)"' : '("[^"]*"|\\S+)';
    last = m.index + m[0].length;
  }
  out += escapeRegExp(phrase.trim().slice(last)) + "$";
  return new RegExp(out);
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/** One place a phrase is used. */
export interface StepUsage {
  uri: string;
  line: number;
  /** "Feature › Scenario", or "Feature › Background" for a step every scenario runs. */
  label: string;
}

/** Where a `# @step` phrase is used, and how many distinct scenarios run it. */
export interface StepUsages {
  /** Scenarios that run the phrase, each counted once whether through the background, their own steps or both; an outline counts per example row. */
  count: number;
  locations: StepUsage[];
}

/** Where the scenarios of the parsed features use a `# @step` phrase. */
export function stepUsages(features: readonly { uri: string; feature: Feature }[], phrase: string): StepUsages {
  const re = stepRegex(phrase);
  const out: StepUsages = { count: 0, locations: [] };
  for (const { uri, feature } of features) {
    const rows = (sc: Scenario) => Math.max(1, sc.examples.reduce((n, e) => n + e.rows.length, 0));
    let inBackground = false;
    for (const st of feature.background?.steps ?? []) {
      if (re.test(st.text)) {
        out.locations.push({ uri, line: st.line, label: `${feature.name} › Background` });
        inBackground = true;
        break;
      }
    }
    for (const sc of feature.scenarios) {
      const own = sc.steps.find((st) => re.test(st.text) || (sc.keyword === "Scenario Outline" && outlineMatches(re, st.text, sc)));
      if (own) {
        out.locations.push({ uri, line: own.line, label: `${feature.name} › ${sc.name}` });
      }
      if (own || inBackground) {
        out.count += rows(sc);
      }
    }
  }
  return out;
}

/** An outline step matches when it matches for any example row's substitution. */
function outlineMatches(re: RegExp, text: string, sc: Scenario): boolean {
  for (const ex of sc.examples) {
    for (const row of ex.rows) {
      let filled = text;
      ex.header.forEach((h, i) => {
        filled = filled.split(`<${h}>`).join(row.cells[i] ?? "");
      });
      if (re.test(filled)) {
        return true;
      }
    }
  }
  return false;
}
