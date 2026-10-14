// llms.txt (llmstxt.org): a markdown map of the site for AI assistants,
// generated from the docs collection at build time so a new page is listed
// without anyone remembering to add it, in the sidebar's groups and order
// (src/llms.ts). slopstopper's llms.txt check asserts the shape (an H1, a
// summary, link sections) and its sitemap check compares the links against
// the pages it can reach.
import type { APIRoute } from 'astro';
import { SUMMARY, docSections, pageURL } from '../llms';

export const GET: APIRoute = async ({ site }) => {
	const base = site ?? new URL('https://apic.sh');
	const lines = [
		'# apic',
		'',
		`> ${SUMMARY}`,
		'',
		'Every page of apic.sh, the documentation for the apic CLI, grouped and ordered as the sidebar has them. Pages link to each other; the CLI reference and the `.http` format guide are the two most agents need, and the agents page is the contract for driving apic from one.',
		'',
		`The whole site as one Markdown document: ${new URL('/llms-full.txt', base).href}`,
		'',
	];
	for (const { heading, pages } of await docSections()) {
		lines.push(`## ${heading}`, '');
		for (const page of pages) {
			const description = page.data.description ? `: ${page.data.description}` : '';
			lines.push(`- [${page.data.title}](${pageURL(page.id, base)})${description}`);
		}
		lines.push('');
	}
	return new Response(lines.join('\n'), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
};
