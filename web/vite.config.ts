import path from "node:path";
import { defineConfig, loadEnv, type ProxyOptions } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const defaultApiProxyTarget = "http://127.0.0.1:8080";

function resolveApiProxyTarget(mode: string): string {
  const env = loadEnv(mode, process.cwd(), "GROWTHOS_WEB_");
  const rawTarget = env.GROWTHOS_WEB_API_PROXY_TARGET || defaultApiProxyTarget;

  let target: URL;
  try {
    target = new URL(rawTarget);
  } catch {
    throw new Error("GROWTHOS_WEB_API_PROXY_TARGET must be a valid HTTP(S) origin");
  }

  if (
    (target.protocol !== "http:" && target.protocol !== "https:") ||
    target.username !== "" ||
    target.password !== "" ||
    target.pathname !== "/" ||
    target.search !== "" ||
    target.hash !== ""
  ) {
    throw new Error(
      "GROWTHOS_WEB_API_PROXY_TARGET must be an HTTP(S) origin without credentials or a path",
    );
  }

  return target.origin;
}

function createApiProxy(target: string): Record<string, ProxyOptions> {
  const options: ProxyOptions = {
    target,
    changeOrigin: true,
  };

  return {
    "^/health$": { ...options },
    "^/ready$": { ...options },
    "^/api(?:/|$)": { ...options },
  };
}

function resolvePort(value: string | undefined, fallback: number): number {
  if (value === undefined || value === "") {
    return fallback;
  }

  const port = Number(value);
  if (!Number.isSafeInteger(port) || port < 1 || port > 65_535) {
    throw new Error("PORT must be an integer between 1 and 65535");
  }
  return port;
}

export default defineConfig(({ mode }) => {
  const apiProxyTarget = resolveApiProxyTarget(mode);

  return {
    base: "/",
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      host: "127.0.0.1",
      port: resolvePort(process.env.PORT, 5173),
      strictPort: true,
      proxy: createApiProxy(apiProxyTarget),
    },
    preview: {
      host: "127.0.0.1",
      port: resolvePort(process.env.PORT, 4173),
      strictPort: true,
      proxy: createApiProxy(apiProxyTarget),
    },
    build: {
      sourcemap: mode === "development",
    },
  };
});
