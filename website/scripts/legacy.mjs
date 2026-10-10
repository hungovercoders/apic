// The docs lived at hungovercoders.github.io/apic/ before apic.sh, and the
// 0.1.x binaries still link there: the `url` in every --json error, the
// SARIF helpUri, and the yaml-language-server modeline `apic init` wrote
// into every project's apic.yaml. docs.yml keeps GitHub Pages serving what
// those need: the schemas as real files (an editor fetches them, so a
// redirect page will not do), and one page per route that forwards to
// apic.sh, keeping the #anchor. Run after `astro build`; writes ./legacy.
import { cpSync, mkdirSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import path from 'node:path';

const site = 'https://apic.sh';
const dist = 'dist';
const out = 'legacy';

function page(route) {
	const target = `${site}${route}`;
	return `<!doctype html>
<meta charset="utf-8">
<title>apic docs have moved</title>
<meta http-equiv="refresh" content="0; url=${target}">
<link rel="canonical" href="${target}">
<script>location.replace(${JSON.stringify(target)} + location.hash)</script>
<p>The apic docs now live at <a href="${target}">${target}</a>.</p>
`;
}

function routes(dir, prefix = '/') {
	const found = [];
	for (const name of readdirSync(dir)) {
		const full = path.join(dir, name);
		if (statSync(full).isDirectory()) {
			found.push(...routes(full, `${prefix}${name}/`));
		} else if (name === 'index.html') {
			found.push(prefix);
		}
	}
	return found;
}

rmSync(out, { recursive: true, force: true });
for (const route of routes(dist)) {
	const dir = path.join(out, route);
	mkdirSync(dir, { recursive: true });
	writeFileSync(path.join(dir, 'index.html'), page(route));
}
writeFileSync(path.join(out, '404.html'), page('/'));
cpSync(path.join(dist, 'schemas'), path.join(out, 'schemas'), { recursive: true });
console.log(`legacy: ${routes(dist).length} redirect pages and the schemas, in ${out}/`);
