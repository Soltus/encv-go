import type { CapacitorConfig } from '@capacitor/cli'

const config: CapacitorConfig = {
  appId: 'com.encvgo.app',
  appName: 'ENCV-go',
  webDir: 'dist',
  server: {
    androidScheme: 'https',
  },
}

export default config
