import { defineConfig, transformWithEsbuild } from "vite";
import react from "@vitejs/plugin-react";

// 项目约定使用 App.js / index.js，因此这里显式让 src 下的 .js 支持 JSX。
function jsxInJs() {
  return {
    name: "jsx-in-js",
    async transform(code, id) {
      if (!id.includes("/src/") && !id.includes("\\src\\")) return null;
      if (!id.endsWith(".js")) return null;
      return transformWithEsbuild(code, id, {
        loader: "jsx",
        jsx: "automatic"
      });
    }
  };
}

export default defineConfig({
  plugins: [jsxInJs(), react()],
  esbuild: {
    loader: "jsx",
    include: /src\/.*\.js$/,
    exclude: []
  },
  optimizeDeps: {
    esbuildCommonjs: { loader: { ".js": "jsx" } }
  },
  server: {
    host: "127.0.0.1",
    port: 4173
  }
});
