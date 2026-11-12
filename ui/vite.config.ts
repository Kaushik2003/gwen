import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

// VITE_GWEN_TARGET=mobile builds the phone app (mobile.html, src/mobile) into dist-mobile, which gwend serves to paired phones.
const mobile = process.env.VITE_GWEN_TARGET === "mobile";

export default defineConfig({
  plugins: mobile ? [react()] : [react(), tailwindcss()],
  publicDir: mobile ? "mobile-public" : "public",
  build: mobile
    ? { outDir: "dist-mobile", emptyOutDir: true, chunkSizeWarningLimit: 1500, rollupOptions: { input: "mobile.html" } }
    : { outDir: "dist", emptyOutDir: true, chunkSizeWarningLimit: 1500 },
});
