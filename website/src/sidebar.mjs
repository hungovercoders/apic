// @ts-check
// The sidebar, in one place: astro.config.mjs hands it to Starlight, and
// llms.txt and llms-full.txt (src/llms.ts) list the pages in the same groups
// and the same order, so the maps an agent reads cannot drift from the
// navigation a person sees.

export const sidebar = [
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
			{ label: 'Privacy', slug: 'privacy' },
			{
				label: 'Architecture',
				link: 'https://github.com/hungovercoders/apic/blob/main/docs/architecture.md',
				attrs: { target: '_blank' },
			},
		],
	},
];

/**
 * The page slugs under one sidebar entry, depth first in sidebar order.
 * External links have no page here, so they are left out.
 * @param {any} entry
 * @returns {string[]}
 */
const slugsOf = (entry) => (entry.slug ? [entry.slug] : (entry.items ?? []).flatMap(slugsOf));

/**
 * The top-level sidebar groups with their page slugs in order, a nested
 * group (Migrate) inside its parent (Guides).
 * @returns {Array<{ label: string, slugs: string[] }>}
 */
export const sidebarGroups = () => sidebar.map((group) => ({ label: group.label, slugs: slugsOf(group) }));
