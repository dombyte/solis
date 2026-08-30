import { defineConfig } from 'vite';
import { yamlPlugin } from 'vite-yaml-plugin';

export default defineConfig({
  base: '/docs/',
  plugins: [
    yamlPlugin(),
  ],
  server: {
    port: 3000,
  },
  build: {
    sourcemap: false,
    minify: true,
    target: 'es2020',
    chunkSizeWarningLimit: 2000, // Increase limit to 2000KB to avoid warning for swagger-ui
  },
  optimizeDeps: {
    include: ['swagger-ui-dist'],
  },
});
