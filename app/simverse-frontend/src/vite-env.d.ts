/// <reference types="vite/client" />

declare module "*.vue" {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

interface ImportMetaEnv {
  readonly VITE_API_BASE: string
  readonly VITE_WS_URL: string
  readonly VITE_GW_PORT: string
  readonly VITE_BASE_URL: string
  readonly DEV: boolean
  readonly PROD: boolean
  readonly MODE: string
  readonly URL: string
  readonly BASE_URL: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
