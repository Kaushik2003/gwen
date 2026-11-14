import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

// VITE_GWEN_TARGET=mobile builds the phone app (mobile.html, src/mobile) into dist-mobile, which gwend serves to paired phones.
const mobile = process.env.VITE_GWEN_TARGET === "mobile";

export default defineConfig({
  plugins: mobile ? [react()] : [react(), tailwindcss()],
  publicDir: mobile ? "mobile-public" : "public",
  // "@/" is src/: the Liquid Charts source (src/components/liquid-charts) imports itself that way.
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  build: mobile
    ? // Every font stays a file: gwend's Content-Security-Policy for the phone allows fonts from 'self' only, not data: URIs.
      { outDir: "dist-mobile", emptyOutDir: true, chunkSizeWarningLimit: 1500, assetsInlineLimit: 0, rollupOptions: { input: "mobile.html" } }
    : { outDir: "dist", emptyOutDir: true, chunkSizeWarningLimit: 1500 },
});
