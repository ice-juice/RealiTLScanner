export function wailsApp() {
  return window.go?.desktop?.App ?? window.go?.main?.App;
}

export function hasWails(): boolean {
  return Boolean(wailsApp()?.StartScan);
}
