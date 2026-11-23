// 开发：npm run dev（5173，/api 代理到 Go 后端 8090）
// 构建：npm run build → ../cmd/server/web/dist，由 Go 用 go:embed 打包进二进制
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [tailwindcss(), react()],
  server: {
    port: 5173,
    proxy: { '/api': { target: process.env.CS_BACKEND_URL ?? 'http://127.0.0.1:8090', changeOrigin: true } },
  },
  build: { outDir: '../cmd/server/web/dist', emptyOutDir: true, sourcemap: false },
});
