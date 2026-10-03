import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const metadata = readFileSync(new URL('../src/context/MetadataContext.tsx', import.meta.url), 'utf8');
const fixtureSocket = readFileSync(new URL('./onboarding-browser/fixtureSocket.ts', import.meta.url), 'utf8');

test('metadata waits for the manifest before either catalog load, with a bounded fallback', () => {
	assert.ok(metadata.includes("import { CATALOG_MANIFEST_WAIT_MS, catalogLoadKey } from '../api/CatalogURL';"));
	assert.ok(metadata.includes('const awaitingManifest = catalogs == null && !manifestWaitExpired;'));
	assert.ok(metadata.includes('setTimeout(() => setManifestWaitExpired(true), CATALOG_MANIFEST_WAIT_MS)'));
	assert.ok(metadata.includes('const catalogKey = catalogLoadKey(locale, catalogs, awaitingManifest);'));
	assert.equal(metadata.match(/if \(catalogKey === null\) return undefined;/g)?.length, 2);
});

test('the onboarding preview socket sends the catalog manifest on connect', () => {
	assert.ok(fixtureSocket.includes("this.push('catalog.changed', catalogManifest())"));
});
