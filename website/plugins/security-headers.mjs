// Writes dist/_headers, the security headers Cloudflare Workers static
// assets (and slopstopper's local server during the browser checks) send
// with every page. public/_headers is the template; this integration fills
// in the Content-Security-Policy's script-src with a hash of every inline
// <script> the build produced (Starlight's theme, search and sidebar
// scripts), so the policy allows exactly those and no other inline script,
// without 'unsafe-inline'. Styles keep 'unsafe-inline': Starlight sets CSS
// custom properties through style attributes, which hashes cannot cover.
import { createHash } from 'node:crypto';
import { readdir, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const PLACEHOLDER = '__INLINE_SCRIPT_HASHES__';
const MAX_HEADER_VALUE = 2000;
const INLINE_SCRIPT = /<script(?:\s[^>]*)?>([\s\S]*?)<\/script>/g;

async function htmlFiles(dir) {
	const out = [];
	for (const entry of await readdir(dir, { withFileTypes: true })) {
		const full = join(dir, entry.name);
		if (entry.isDirectory()) out.push(...(await htmlFiles(full)));
		else if (entry.name.endsWith('.html')) out.push(full);
	}
	return out;
}

async function inlineScriptHashes(dir) {
	const hashes = new Set();
	for (const file of await htmlFiles(dir)) {
		const html = await readFile(file, 'utf8');
		for (const match of html.matchAll(INLINE_SCRIPT)) {
			const openTag = match[0].slice(0, match[0].indexOf('>') + 1);
			if (/\ssrc=/.test(openTag)) continue;
			const digest = createHash('sha256').update(match[1], 'utf8').digest('base64');
			hashes.add(`'sha256-${digest}'`);
		}
	}
	return [...hashes].sort();
}

export default function securityHeaders() {
	return {
		name: 'security-headers',
		hooks: {
			'astro:build:done': async ({ dir, logger }) => {
				const dist = fileURLToPath(dir);
				const template = await readFile(join(dist, '_headers'), 'utf8');
				const occurrences = template.split(PLACEHOLDER).length - 1;
				if (occurrences !== 1) {
					throw new Error(`public/_headers must carry ${PLACEHOLDER} exactly once, on the Content-Security-Policy line; found ${occurrences}`);
				}
				const hashes = await inlineScriptHashes(dist);
				const built = template.replace(PLACEHOLDER, hashes.join(' '));
				// Cloudflare ignores a _headers value longer than 2,000 characters, and
				// nothing in CI would notice: the local server has no such limit.
				const policy = built.split('\n').find((line) => line.trimStart().startsWith('Content-Security-Policy:')) ?? '';
				const value = policy.slice(policy.indexOf(':') + 1).trim();
				if (value.length > MAX_HEADER_VALUE) {
					throw new Error(`Content-Security-Policy is ${value.length} characters; Cloudflare drops a _headers value over ${MAX_HEADER_VALUE}. Fewer distinct inline scripts, or a shorter policy.`);
				}
				await writeFile(join(dist, '_headers'), built);
				logger.info(`_headers: script-src allows ${hashes.length} inline script hash(es); the policy is ${value.length} of ${MAX_HEADER_VALUE} characters`);
			},
		},
	};
}
