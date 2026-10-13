// llms.txt (llmstxt.org): a markdown map of the site for AI assistants,
// generated from the docs collection at build time so a new page is listed
// without anyone remembering to add it. slopstopper's llms.txt check
// asserts the shape (an H1, a summary, link sections) and its sitemap
// check compares the links against the pages it can reach.
import type { APIRoute } from 'astro';
import { getCollection } from 'astro:content';

const SUMMARY =
	'apic is epic: run API requests from plain .http files, in the terminal, in CI, or from an AI agent, on any platform, with one static binary.';

const sections: Array<[string, (id: string) => boolean]> = [
	['Course: from zero to apic', (id) => id === 'learn' || id.startsWith('learn/')],
	['Migration guides', (id) => id.startsWith('migrate/')],
	['Docs', () => true],
];

export const GET: APIRoute = async ({ site }) => {
	// Starlight builds no route for a draft, so it gets no link either.
	const docs = (await getCollection('docs', ({ data }) => !data.draft)).sort((a, b) => a.id.localeCompare(b.id));
	const url = (id: string) => new URL(id === 'index' ? '/' : `/${id}/`, site).href;
	const lines = ['# apic', '', `> ${SUMMARY}`, '', 'Every page of apic.sh, the documentation for the apic CLI. Pages link to each other; the CLI reference and the `.http` format guide are the two most agents need.', ''];
	const seen = new Set<string>();
	for (const [heading, matches] of sections) {
		const pages = docs.filter((d) => !seen.has(d.id) && matches(d.id));
		if (pages.length === 0) continue;
		lines.push(`## ${heading}`, '');
		for (const page of pages) {
			seen.add(page.id);
			const description = page.data.description ? `: ${page.data.description}` : '';
			lines.push(`- [${page.data.title}](${url(page.id)})${description}`);
		}
		lines.push('');
	}
	return new Response(lines.join('\n'), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
};
