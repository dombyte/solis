import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import license from "rollup-plugin-license"
import { defineConfig } from "vite"
import { copyFileSync, existsSync, mkdirSync } from "fs"

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    license({
      thirdParty: {
        includePrivate: false,
        includeSelf: false,
        output: {
          file: path.join(__dirname, "public", "data", "licenses.json"),
          encoding: "utf-8",
          template: (dependencies) => {
            return JSON.stringify(
              dependencies.map((dep) => {
                // Extract repository URL - can be string or object
                let repoUrl: string;
                if (typeof dep.repository === 'string') {
                  repoUrl = dep.repository;
                } else if (dep.repository && typeof dep.repository === 'object') {
                  repoUrl = (dep.repository as any).url || `https://www.npmjs.com/package/${dep.name}`;
                } else {
                  repoUrl = `https://www.npmjs.com/package/${dep.name}`;
                }
                // Clean up .git suffix from git URLs
                repoUrl = repoUrl.replace(/\.git$/, '');

                return {
                  name: dep.name,
                  version: dep.version,
                  license: dep.license || "Unknown",
                  repository: repoUrl,
                };
              }),
              null,
              2
            );
          },
        },
      },
    }),
   // Copy data files to dist after build so they're available
    {
      name: 'copy-data-to-dist',
      apply: 'build',
      closeBundle: () => {
        const dataDir = path.join(__dirname, 'public', 'data');
        const destDir = path.join(__dirname, 'dist', 'data');

        // Ensure dist/data directory exists
        if (!existsSync(destDir)) {
          mkdirSync(destDir, { recursive: true });
        }

        // Copy all files from public/data to dist/data
        const files = ['licenses.json', 'version.json'];
        files.forEach(file => {
          const src = path.join(dataDir, file);
          const dest = path.join(destDir, file);
          if (existsSync(src)) {
            copyFileSync(src, dest);
          }
        });
      },
    },
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 3000,
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true },
      "/ws": { target: "ws://localhost:8080", ws: true },
    },
  },
  preview: {
    port: 4174,
    allowedHosts: ['dev.stratopi.welan'],
    },

  build: {
    outDir: "dist",
    emptyOutDir: true,
    minify: true,
    rollupOptions: {
      output: {
        chunkFileNames: "assets/[name]-[hash].js",
        entryFileNames: "assets/[name]-[hash].js",
        assetFileNames: "assets/[name]-[hash].[ext]",
        codeSplitting: {
          groups: [
            {
              name: 'react-vendor',
              test: /node_modules[\\/]react/,
              priority: 20,
            },
            {
              name: 'ui-vendor',
              test: /node_modules[\\/]@radix/,
              priority: 15,
            },
            {
              name: 'chart-vendor',
              test: /node_modules[\\/]chart.js/,
              priority: 14,
            },
            {
              name: 'date-vendor',
              test: /node_modules[\\/]date-fns/,
              priority: 13,
            },
            {
              name: 'icon-vendor',
              test: /node_modules[\\/]lucide-react/,
              priority: 12,
            },
            {
              name: 'vendor',
              test: /node_modules/,
              priority: 10,
            },
            {
              name: 'common',
              minShareCount: 2,
              minSize: 10000,
              priority: 5,
            },
            {
              name: 'index',
              test: /src/,
              priority: 11,
            },
          ],
        },
      },
    },
  },
})
