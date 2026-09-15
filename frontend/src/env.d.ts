/// <reference types="vite/client" />

declare module "*.vue" {
  import type { DefineComponent } from "vue";
  const component: DefineComponent<object, object, unknown>;
  export default component;
}

interface WailsApp {
  StartScan: (req: unknown) => Promise<void>;
  StopScan: () => Promise<void>;
  GetDefaults: () => Promise<unknown>;
  ValidateConfig: (req: unknown) => Promise<unknown>;
  PickOutputFile: () => Promise<string>;
  PickInputFile: () => Promise<string>;
  OpenOutputFolder: (path: string) => Promise<void>;
  GeoDBStatus: () => Promise<unknown>;
  DownloadGeoDB: () => Promise<void>;
  Theme: () => Promise<string>;
  SetTheme: (theme: string) => Promise<void>;
}

interface Window {
  runtime?: {
    EventsOn(event: string, cb: (data: unknown) => void): void;
  };
  go?: {
    desktop?: {
      App?: WailsApp;
    };
    main?: {
      App?: WailsApp;
    };
  };
}
