// llms-full.txt (llmstxt.org): every page of the site as one markdown
// document, for an agent that wants the whole manual in a single fetch
// rather than one page at a time. It is generated from the docs collection
// at build time, like llms.txt, so a new page is included without anyone
// remembering to add it. Each page starts with its title, its canonical
// URL and its description; the body is the page's source Markdown, with
// the relative links pages write to each other (`format.md#anchor`)
// rewritten to the URLs the site serves.
import type { APIRoute } from 'astro';
import { getCollection } from 'astro:content';

const SUMMARY =
	'apic is epic: run API requests from plain .http files, in the terminal, in CI, or from an AI agent, on any platform, with one static binary.';

const order = (id: string) => (id === 'index' ? 0 : id.startsWith('learn') ? 2 : id.startsWith('migrate/') ? 3 : 1);

export const GET: APIRoute = async ({ site }) => {
	const docs = (await getCollection('docs', ({ data }) => !data.draft)).sort(
		(a, b) => order(a.id) - order(b.id) || a.id.localeCompare(b.id),
	);
	const url = (id: string) => new URL(id === 'index' ? '/' : `/${id}/`, site).href;
	// A page's links are written as `format.md#anchor` or `learn/index.md`,
	// the way GitHub renders them (see docs/docs-site.md); serve them as
	// the site does. Links to files under public/ (`assets/x.svg`,
	// `schemas/x.json`) are already the path the site serves.
	// Links resolve against the page's file, not its route: learn/index.md
	// serves at /learn/ but links to its siblings as `01-first-request.md`.
	const resolve = (file: string, body: string) =>
		body.replace(/\]\(([^)\s:]+?\.md)(#[^)]*)?\)/g, (_, target: string, hash = '') => {
			const rel = file.replace(/^.*?src\/content\/docs\//, '');
			const dir = rel.includes('/') ? rel.slice(0, rel.lastIndexOf('/') + 1) : '';
			const path = new URL(dir + target, 'https://x/').pathname.slice(1);
			const id = path.replace(/\.md$/, '').replace(/(^|\/)index$/, '');
			return `](${new URL(id === '' ? '/' : `/${id}/`, site).href}${hash})`;
		});
	const lines = ['# apic', '', `> ${SUMMARY}`, '', 'Every page of apic.sh, the documentation for the apic CLI, in one document. The short index is at /llms.txt.', ''];
	for (const page of docs) {
		lines.push('---', '', `# ${page.data.title}`, '', `URL: ${url(page.id)}`);
		if (page.data.description) lines.push('', page.data.description);
		lines.push('', resolve(page.filePath ?? page.id, page.body ?? '').trim(), '');
	}
	return new Response(lines.join('\n'), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
};
