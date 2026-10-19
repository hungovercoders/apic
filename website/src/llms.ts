// What llms.txt and llms-full.txt (src/pages/) share: the docs pages grouped
// and ordered as the sidebar has them, and the page bodies as plain
// Markdown with links an agent can follow.
import { getCollection, type CollectionEntry } from 'astro:content';
import { sidebarGroups } from './sidebar.mjs';

export const SUMMARY =
	'apic is epic: run API requests from plain .http files, in the terminal, in CI, or from an AI agent, on any platform, with one static binary.';

type Doc = CollectionEntry<'docs'>;

/**
 * The pages in the sidebar's groups and order (src/sidebar.mjs), the landing
 * page first in Start. A page the sidebar does not list still appears, in a
 * last group, so a page added without a sidebar entry is never dropped.
 */
export async function docSections(): Promise<Array<{ heading: string; pages: Doc[] }>> {
	// Starlight builds no route for a draft, so it gets no entry either.
	const docs = await getCollection('docs', ({ data }) => !data.draft);
	const byId = new Map(docs.map((d) => [d.id, d]));
	const seen = new Set<string>();
	const take = (ids: string[]) =>
		ids.flatMap((id) => {
			const page = byId.get(id);
			if (!page || seen.has(id)) return [];
			seen.add(id);
			return [page];
		});
	const sections = sidebarGroups().map((group, i) => ({
		heading: group.label,
		pages: take(i === 0 ? ['index', ...group.slugs] : group.slugs),
	}));
	const rest = docs.filter((d) => !seen.has(d.id)).sort((a, b) => a.id.localeCompare(b.id));
	if (rest.length > 0) sections.push({ heading: 'Other pages', pages: rest });
	return sections.filter((s) => s.pages.length > 0);
}

/** The URL the site serves a page at. */
export const pageURL = (id: string, site: URL) => new URL(id === 'index' ? '/' : `/${id}/`, site).href;

/**
 * A page's source as plain Markdown for an agent.
 *
 * Pages link to each other as `format.md#anchor` (see docs/docs-site.md);
 * those become the URLs the site serves, resolved against the page's file,
 * not its route: learn/index.md serves at /learn/ but links to its siblings
 * as `01-first-request.md`. The two MDX pages use Starlight components; each
 * becomes the Markdown it stands for, so no JSX reaches the reader.
 */
export function pageMarkdown(page: Doc, site: URL): string {
	const file = page.filePath ?? page.id;
	let body = page.body ?? '';
	if (file.endsWith('.mdx')) body = mdxToMarkdown(body, site);
	const rel = file.replace(/^.*?src\/content\/docs\//, '');
	const dir = rel.includes('/') ? rel.slice(0, rel.lastIndexOf('/') + 1) : '';
	return body
		.replace(/\]\(([^)\s:]+?\.md)(#[^)]*)?\)/g, (_, target: string, hash = '') => {
			const path = new URL(dir + target, 'https://x/').pathname.slice(1);
			const id = path.replace(/\.md$/, '').replace(/(^|\/)index$/, '');
			return `](${pageURL(id === '' ? 'index' : id, site)}${hash})`;
		})
		.trim();
}

const attr = (tag: string, name: string) => tag.match(new RegExp(`\\b${name}="([^"]*)"`))?.[1] ?? '';

/**
 * The Starlight components the MDX pages use, as Markdown: a tab becomes a
 * bold label over its content, a link card a list item, and the imports,
 * wrappers and MDX comments go. A component this does not know is left as
 * it is, so a new one shows up in the output rather than vanishing.
 */
function mdxToMarkdown(body: string, site: URL): string {
	return body
		.replace(/^import\s.*$/gm, '')
		.replace(/\{\/\*[\s\S]*?\*\/\}/g, '')
		.replace(/^<\/?(Tabs|CardGrid)>\s*$/gm, '')
		.replace(/^<\/TabItem>\s*$/gm, '')
		.replace(/^<TabItem\b[^>]*>\s*$/gm, (tag) => `**${attr(tag, 'label')}:**`)
		.replace(/^<LinkCard\b[^>]*\/>\s*$/gm, (tag) => {
			const href = new URL(attr(tag, 'href'), site).href;
			return `- [${attr(tag, 'title')}](${href}): ${attr(tag, 'description')}`;
		})
		.replace(/\n{3,}/g, '\n\n');
}
