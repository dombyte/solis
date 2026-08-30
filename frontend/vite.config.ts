import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { VitePWA } from "vite-plugin-pwa"
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
   // Copy licenses.json to dist after build so it's available
    {
      name: 'copy-licenses-to-dist',
      apply: 'build',
      closeBundle: () => {
        const src = path.join(__dirname, 'public', 'data', 'licenses.json');
        const destDir = path.join(__dirname, 'dist', 'data');
        const dest = path.join(destDir, 'licenses.json');

        // Ensure dist/data directory exists
        if (!existsSync(destDir)) {
          mkdirSync(destDir, { recursive: true });
        }

        // Copy file if it exists
        if (existsSync(src)) {
          copyFileSync(src, dest);
        }
      },
    },
    VitePWA({
      manifest: {
        name: "Solis Monitor",
        short_name: "Solis",
        description: "Monitor your Solis inverter",
        theme_color: "#000000",
        background_color: "#000000",
        display: "standalone",
      },
      includeAssets: ["favicon.svg"],
      strategies: "generateSW",
      // Use manual registration with workbox-window
      injectRegister: false,
      pwaAssets: {
        disabled: false,
        preset: "minimal-2023",
        image: "public/favicon.svg",
      },
      workbox: {
        clientsClaim: true,
        skipWaiting: false,
        globPatterns: [
          "**/*.{js,css,html,svg,png,jpg,jpeg,webp,woff2,ttf,eot,json,ico}"
        ],
        runtimeCaching: [],
        navigateFallback: "/index.html",
        navigateFallbackDenylist: [
          /^\/api\//,
          /^\/docs/,
          /^\/health/,
          /^\/metrics/,
          /^\/ws/,
          /\.\w+$/,
        ],
      },
      // Enable workbox-window for better update control
      selfDestroying: false,
    }),
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
