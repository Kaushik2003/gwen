import "@fontsource-variable/geist";
import "@fontsource-variable/geist-mono";
import "@fontsource-variable/inter";
import "@fontsource-variable/sofia-sans-semi-condensed";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { LiquidProvider } from "./components/liquid-charts/liquid-provider";
import "./index.css";
import { applyStoredTheme } from "./theme";

applyStoredTheme();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <LiquidProvider>
      <App />
    </LiquidProvider>
  </StrictMode>,
);
