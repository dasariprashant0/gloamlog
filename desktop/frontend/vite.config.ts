import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
    plugins: [react()],
    // @personal/ui is a file: symlink; keep it under node_modules so it uses this app's React.
    resolve: {preserveSymlinks: true},
    // Two pages: the main window and the quick-jot panel.
    build: {rollupOptions: {input: {main: 'index.html', jot: 'jot.html'}}},
    server: {port: 9245, strictPort: true},
});
