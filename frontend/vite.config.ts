import { defineConfig } from 'vite'
import wails from '@wailsio/runtime/plugins/vite'

export default defineConfig({ clearScreen: false, plugins: [wails('./bindings')], server: { strictPort: true, port: 9245 } })
