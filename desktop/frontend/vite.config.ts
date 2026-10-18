import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
import {fileURLToPath} from 'node:url';

export default defineConfig(({mode}) => ({
    plugins: [react()],
    resolve: {
        // @personal/ui is a file: symlink; keep it under node_modules so it uses this app's React.
        preserveSymlinks: true,
        // `npm run dev:mock`: run the UI in a plain browser against src/dev/mock.ts instead of the Go services.
        alias: mode === 'mock'
            ? [{find: /^.*\/bindings\/github\.com\/dasariprashant0\/gloamlog\/desktop$/, replacement: fileURLToPath(new URL('./src/dev/mock.ts', import.meta.url))}]
            : [],
    },
    // Three pages: the main window, the quick-jot panel and the Settings window.
    build: {rollupOptions: {input: {main: 'index.html', jot: 'jot.html', settings: 'settings.html'}}},
    // 127.0.0.1, not "localhost": Node binds localhost to IPv6 (::1) and the Wails dev proxy dials IPv4.
    server: {host: '127.0.0.1', port: mode === 'mock' ? 9246 : 9245, strictPort: true},
}));
