// llms-full.txt (llmstxt.org): every page of the site as one Markdown
// document, for an agent that wants the whole manual in a single fetch
// rather than one page at a time. It is generated from the docs collection
// at build time, like llms.txt and in the same order (src/llms.ts), so a new
// page is included without anyone remembering to add it. Each page starts
// with its title, its canonical URL and its description; the body is the
// page's source as plain Markdown, MDX components converted and the
// relative links pages write to each other rewritten to the URLs the site
// serves.
import type { APIRoute } from 'astro';
import { SUMMARY, docSections, pageMarkdown, pageURL } from '../llms';

export const GET: APIRoute = async ({ site }) => {
	const base = site ?? new URL('https://apic.sh');
	const lines = [
		'# apic',
		'',
		`> ${SUMMARY}`,
		'',
		`Every page of apic.sh, the documentation for the apic CLI, in one document, in the sidebar's order. The short index is at ${new URL('/llms.txt', base).href}.`,
		'',
	];
	for (const { pages } of await docSections()) {
		for (const page of pages) {
			lines.push('---', '', `# ${page.data.title}`, '', `URL: ${pageURL(page.id, base)}`);
			if (page.data.description) lines.push('', page.data.description);
			lines.push('', pageMarkdown(page, base), '');
		}
	}
	return new Response(lines.join('\n'), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
};
