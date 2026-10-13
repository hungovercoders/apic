// Starlight route middleware: the share tags Starlight does not emit on
// its own. Starlight writes og:title, og:description and twitter:card for
// every page; the Twitter title and description, and the site-wide image
// (public/og-image.png, 1200×630), are added here so a link to any page
// unfurls the same way everywhere. slopstopper's SEO check asserts all
// four on every page it audits.
import { defineRouteMiddleware } from '@astrojs/starlight/route-data';

export const onRequest = defineRouteMiddleware((context) => {
	const { head, entry } = context.locals.starlightRoute;
	const meta = (key: 'property' | 'name', value: string) =>
		head.find((tag) => tag.tag === 'meta' && tag.attrs?.[key] === value)?.attrs?.content;
	const image = new URL('/og-image.png', context.site).href;
	const title = String(meta('property', 'og:title') ?? entry.data.title);
	const description = String(meta('property', 'og:description') ?? entry.data.description ?? '');
	head.push(
		{ tag: 'meta', attrs: { property: 'og:image', content: image } },
		{ tag: 'meta', attrs: { name: 'twitter:title', content: title } },
		{ tag: 'meta', attrs: { name: 'twitter:description', content: description } },
		{ tag: 'meta', attrs: { name: 'twitter:image', content: image } },
	);
});
