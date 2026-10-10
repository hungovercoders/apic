// Pages link to each other the way they do on GitHub, `format.md#anchor`,
// and to files under public/ by their path from the content root,
// `assets/apic-ui.svg`. This rewrites both to the URLs the site serves:
// `/format/#anchor` and `/assets/apic-ui.svg`. scripts/skilldocs in the
// repository root relies on the `.md` form to make the skill's links
// absolute, so keep writing links that way.
import path from 'node:path';
import { visit } from 'unist-util-visit';

const contentRoot = path.resolve('src/content/docs');

export default function remarkDocLinks() {
	return (tree, file) => {
		const from = path.dirname(path.relative(contentRoot, file.path));
		visit(tree, ['link', 'image', 'definition'], (node) => {
			const url = node.url;
			if (!url || /^(?:[a-z]+:|\/|#)/i.test(url)) return;
			const [target, hash = ''] = url.split('#');
			const resolved = path.posix.normalize(path.posix.join(from, target));
			if (resolved.endsWith('.md') || resolved.endsWith('.mdx')) {
				let page = resolved.replace(/\.mdx?$/, '');
				if (page === 'index') page = '';
				else if (page.endsWith('/index')) page = page.slice(0, -'index'.length);
				else page += '/';
				node.url = '/' + page + (hash ? '#' + hash : '');
			} else {
				node.url = '/' + resolved + (hash ? '#' + hash : '');
			}
		});
	};
}
