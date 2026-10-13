// robots.txt, generated from the site URL at build time so the Sitemap
// pointer follows `site` in astro.config.mjs (Starlight writes
// sitemap-index.xml). slopstopper's robots.txt check asserts the file is
// reachable, allows crawling and names a sitemap.
import type { APIRoute } from 'astro';

export const GET: APIRoute = ({ site }) => {
	const sitemap = new URL('/sitemap-index.xml', site).href;
	return new Response(`User-agent: *\nAllow: /\n\nSitemap: ${sitemap}\n`, {
		headers: { 'Content-Type': 'text/plain; charset=utf-8' },
	});
};
