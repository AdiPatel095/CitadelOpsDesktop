import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

const fixtureRoot = fileURLToPath(new URL('.', import.meta.url));

export default defineConfig({
	root: fixtureRoot,
	plugins: [react(), tailwindcss()],
	resolve: {
		alias: [
			{ find: '../../api/ApiContext', replacement: fileURLToPath(new URL('./api-context.mock.tsx', import.meta.url)) },
			{ find: '../../context/MetadataContext', replacement: fileURLToPath(new URL('./metadata-context.mock.tsx', import.meta.url)) },
		],
	},
	server: {
		host: '127.0.0.1',
		port: 41733,
		strictPort: true,
		proxy: {
			'^/api/v2/equipment/optimize': 'http://127.0.0.1:41732',
			'^/fixture/health': 'http://127.0.0.1:41732',
		},
	},
	build: {
		outDir: fileURLToPath(new URL('./dist', import.meta.url)),
		emptyOutDir: true,
	},
});
