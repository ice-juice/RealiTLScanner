export type TargetKind = "addr" | "file" | "url";
export type TargetScope = "nearby" | "prefix" | "asn";
export type ScanState =
  | "idle"
  | "planning"
  | "scanning"
  | "stopping"
  | "completed"
  | "cancelled"
  | "failed";

export interface ScanRequest {
  targetKind: TargetKind;
  targetValue: string;
  port: number;
  scanThreads: number;
  probeThreads: number;
  timeoutSec: number;
  probeTimeoutSec: number;
  scope: TargetScope;
  limit: number;
  asnMaxPrefixes: number;
  enableIPv6: boolean;
  disablePortProbe: boolean;
  verifySni: boolean;
  verbose: boolean;
  outputPath: string;
  geoDBPath: string;
}

export interface Progress {
  Generated: number;
  Probed: number;
  ProbePassed: number;
  Scanned: number;
  Found: number;
  Elapsed: number;
  Rate: number;
  Total: number;
}

export interface Result {
  ip: string;
  origin: string;
  tls: string;
  alpn: string;
  curve: string;
  certLength: string;
  certSignature: string;
  certPublicKey: string;
  certDomain: string;
  certIssuer: string;
  geoCode: string;
}

export interface LogEntry {
  Time: string;
  Level: string;
  Message: string;
  Fields?: Record<string, string>;
}

export interface GeoStatus {
  installed: boolean;
  path: string;
  modTime: string;
}

export interface FieldError {
  field: string;
  message: string;
}

export const defaultRequest = (): ScanRequest => ({
  targetKind: "addr",
  targetValue: "",
  port: 443,
  scanThreads: 100,
  probeThreads: 100,
  timeoutSec: 10,
  probeTimeoutSec: 1,
  scope: "prefix",
  limit: 4096,
  asnMaxPrefixes: 32,
  enableIPv6: false,
  disablePortProbe: false,
  verifySni: true,
  verbose: false,
  outputPath: "",
  geoDBPath: "",
});
