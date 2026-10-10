// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLinksValidator from 'starlight-links-validator';
import { unified } from '@astrojs/markdown-remark';
import remarkDocLinks from './plugins/remark-doc-links.mjs';

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
		starlight({
			title: 'apic',
			description:
				'apic is epic: run .http request files from the terminal, CI or an AI agent, on any platform.',
			logo: { src: './src/assets/logo.svg' },
			favicon: '/favicon.svg',
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/hungovercoders/apic' },
			],
			editLink: {
				baseUrl: 'https://github.com/hungovercoders/apic/edit/main/website/src/content/docs/',
			},
			customCss: ['./src/styles/custom.css'],
			// Internal links and anchors are checked at build time, which is
			// what `mkdocs build --strict` used to do.
			plugins: [starlightLinksValidator()],
			sidebar: [
				{
					label: 'Start',
					items: [
						{ label: 'Getting started', slug: 'getting-started' },
						{ label: 'Terminal UI', slug: 'tui' },
						{ label: 'Cheat sheet', slug: 'cheatsheet' },
					],
				},
				{
					label: 'Learn',
					collapsed: true,
					items: [
						{ label: 'From zero to apic', slug: 'learn' },
						{ label: '0. What apic is and why', slug: 'learn/00-what-is-apic' },
						{ label: '1. Install and send your first request', slug: 'learn/01-first-request' },
						{ label: '2. Variables and environments', slug: 'learn/02-variables-and-environments' },
						{ label: '3. Capture, the session and flows', slug: 'learn/03-capture-session-flows' },
						{ label: '4. Assertions, validation and polling', slug: 'learn/04-assertions' },
						{ label: '5. The terminal UI tour', slug: 'learn/05-terminal-ui' },
						{ label: '6. Authentication', slug: 'learn/06-authentication' },
						{ label: '7. Behaviour tests with Gherkin', slug: 'learn/07-gherkin' },
						{ label: '8. CI without leaking secrets', slug: 'learn/08-ci' },
						{ label: '9. From OpenAPI to a project, and back to curl', slug: 'learn/09-import-export' },
						{ label: '10. Agents and MCP', slug: 'learn/10-agents' },
						{ label: '11. Real APIs and the Taskfile front door', slug: 'learn/11-real-apis' },
						{ label: '12. Editors', slug: 'learn/12-editors' },
						{ label: '13. How apic works inside, and contributing', slug: 'learn/13-inside-apic' },
					],
				},
				{
					label: 'Guides',
					items: [
						{ label: 'The .http format', slug: 'format' },
						{ label: 'Authentication', slug: 'auth' },
						{ label: 'Testing with Gherkin', slug: 'testing' },
						{ label: 'Cookbook', slug: 'cookbook' },
						{ label: 'Agents and MCP', slug: 'agents' },
						{ label: 'Editors', slug: 'editors' },
						{ label: 'Taskfile', slug: 'taskfile' },
						{
							label: 'Migrate',
							items: [
								{ label: 'From Postman', slug: 'migrate/postman' },
								{ label: 'From Bruno', slug: 'migrate/bruno' },
								{ label: 'From Hurl', slug: 'migrate/hurl' },
								{ label: 'From httpyac', slug: 'migrate/httpyac' },
								{ label: 'From curl and Taskfile', slug: 'migrate/curl' },
							],
						},
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'CLI reference', slug: 'cli' },
						{ label: 'Errors', slug: 'errors' },
						{ label: 'FAQ', slug: 'faq' },
						{ label: 'Comparison', slug: 'comparison' },
						{ label: 'Verifying a release', slug: 'verifying' },
						{ label: 'Licence', slug: 'licence' },
						{
							label: 'Architecture',
							link: 'https://github.com/hungovercoders/apic/blob/main/docs/architecture.md',
							attrs: { target: '_blank' },
						},
					],
				},
			],
		}),
	],
});
