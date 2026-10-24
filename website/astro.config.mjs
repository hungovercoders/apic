// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLinksValidator from 'starlight-links-validator';
import { unified } from '@astrojs/markdown-remark';
import remarkDocLinks from './plugins/remark-doc-links.mjs';
import securityHeaders from './plugins/security-headers.mjs';
import { sidebar } from './src/sidebar.mjs';

// The page paths and heading anchors here are a public contract: the
// binary, the VS Code extension and the schema ids link to them (see
// AGENTS.md, Docs site). Renaming a page or a heading needs the same care
// as renaming a flag.
export default defineConfig({
	site: 'https://apic.sh',
	trailingSlash: 'always',
	markdown: {
		processor: unified({ remarkPlugins: [remarkDocLinks] }),
	},
	integrations: [
		securityHeaders(),
		starlight({
			title: 'apic',
			// Share tags for every page (src/routeData.ts); robots.txt and
			// llms.txt are generated under src/pages/.
			routeMiddleware: './src/routeData.ts',
			description:
				'apic is epic: run .http request files from the terminal, CI or an AI agent, on any platform.',
			logo: { src: './src/assets/logo.svg' },
			favicon: '/favicon.svg',
			// Point agents and tools at the plain-text maps (src/pages/llms*.txt.ts).
			head: [
				{ tag: 'link', attrs: { rel: 'alternate', type: 'text/plain', title: 'llms.txt', href: '/llms.txt' } },
				{ tag: 'link', attrs: { rel: 'alternate', type: 'text/plain', title: 'llms-full.txt', href: '/llms-full.txt' } },
				// The colour a phone's browser paints its own bar: Starlight's header
				// background (--sl-color-bg-nav) in each scheme, so the bar does not
				// sit as a pale strip above a dark header. Media queries follow the
				// OS, as the picker's default does; a script would cost a CSP hash.
				{ tag: 'meta', attrs: { name: 'theme-color', media: '(prefers-color-scheme: light)', content: '#f6f7f9' } },
				{ tag: 'meta', attrs: { name: 'theme-color', media: '(prefers-color-scheme: dark)', content: '#23262f' } },
			],
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/hungovercoders/apic' },
			],
			editLink: {
				baseUrl: 'https://github.com/hungovercoders/apic/edit/main/website/src/content/docs/',
			},
			customCss: ['./src/styles/custom.css'],
			// Starlight's footer plus the privacy line (src/components/Footer.astro).
			components: { Footer: './src/components/Footer.astro' },
			// Internal links and anchors are checked at build time, which is
			// what `mkdocs build --strict` used to do.
			plugins: [starlightLinksValidator()],
			// src/sidebar.mjs, which llms.txt and llms-full.txt also read.
			sidebar,
		}),
	],
});
