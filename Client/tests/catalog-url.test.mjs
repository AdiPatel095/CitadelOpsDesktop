import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

// CIT-35: collection URLs name the content digest so the worker can serve them as immutable.
// The same test runs in the desktop client and the hosted command center (only API_ROOT differs).
const API_ROOT = '/src/api';
const vite = await createServer({
	root: fileURLToPath(new URL('..', import.meta.url)),
	appType: 'custom', logLevel: 'silent', server: { middlewareMode: true },
});
const originalFetch = globalThis.fetch;
after(async () => {
	globalThis.fetch = originalFetch;
	await vite.close();
});
const { catalogPath, manifestDigest } = await vite.ssrLoadModule(`${API_ROOT}/CatalogURL.ts`);
const { CitadelAPI: client } = await vite.ssrLoadModule(`${API_ROOT}/CitadelClient.ts`);

const DIGEST = 'a'.repeat(64);

test('a collection URL carries the locale and the digest, each only when known', () => {
	assert.equal(catalogPath('units'), '/api/v2/game-data/units');
	assert.equal(catalogPath('units', 'de'), '/api/v2/game-data/units?locale=de');
	assert.equal(catalogPath('units', undefined, DIGEST), `/api/v2/game-data/units?digest=${DIGEST}`);
	assert.equal(catalogPath('units', 'de', DIGEST), `/api/v2/game-data/units?locale=de&digest=${DIGEST}`);
	assert.equal(catalogPath('units', '', ''), '/api/v2/game-data/units');
	assert.equal(catalogPath('construction-item-icons', ' fr '), '/api/v2/game-data/construction-item-icons?locale=fr');
	assert.equal(catalogPath('a b/c'), '/api/v2/game-data/a%20b%2Fc', 'the name is encoded');
});

test('different locales and different digests are different URLs, so they never share a cache entry', () => {
	const urls = new Set([
		catalogPath('units', 'en', DIGEST), catalogPath('units', 'de', DIGEST), catalogPath('units', 'en', 'b'.repeat(64)), catalogPath('units', undefined, DIGEST),
	]);
	assert.equal(urls.size, 4);
});

test('the manifest digest is read defensively', () => {
	assert.equal(manifestDigest({ metadata: { digestSha256: ` ${DIGEST} ` }, catalogs: [] }), DIGEST);
	for (const value of [null, undefined, 'x', [], {}, { metadata: null }, { metadata: {} }, { metadata: { digestSha256: 5 } }]) assert.equal(manifestDigest(value), '');
});

test('the client asks with the digest once a manifest names it, and follows a new digest without a reload', async () => {
	const requested = [];
	globalThis.fetch = async (input) => {
		requested.push(String(input));
		const body = String(input).endsWith('/game-data') ? { metadata: { digestSha256: DIGEST }, catalogs: [] } : { metadata: {}, catalog: {}, items: [] };
		return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
	};
	await client.getCatalog('units', 'en');
	assert.match(requested.at(-1), /\/game-data\/units\?locale=en$/, 'no digest before a manifest');
	await client.getCatalogManifest();
	await client.getCatalog('units', 'en');
	assert.match(requested.at(-1), new RegExp(`/game-data/units\\?locale=en&digest=${DIGEST}$`), 'the manifest\'s digest names the URL');
	const next = 'c'.repeat(64);
	client.noteCatalogManifest({ metadata: { digestSha256: next }, catalogs: [] });
	await client.getCatalog('units', 'en');
	assert.match(requested.at(-1), new RegExp(`digest=${next}$`), 'a catalog.changed manifest moves the key to the new content');
	client.noteCatalogManifest({ metadata: {}, catalogs: [] });
	await client.getCatalog('units');
	assert.match(requested.at(-1), /\/game-data\/units$/, 'a worker without a digest is asked plainly');
});
