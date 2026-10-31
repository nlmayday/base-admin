import { http } from "@/utils/http";

export type ServerStats = {
  collectedAt: string;
  host: {
    hostname: string;
    os: string;
    platform: string;
    platformVersion: string;
    kernelVersion: string;
    kernelArch: string;
    uptimeSeconds: number;
    bootTime: string;
    procs: number;
  };
  cpu: {
    modelName: string;
    logicalCores: number;
    physicalCores: number;
    percent: number;
    perCore: number[];
  };
  /** 平台不支持时为 null */
  load: { load1: number; load5: number; load15: number } | null;
  memory: MemoryStats;
  swap: MemoryStats;
  disks: DiskStats[];
};

export type MemoryStats = {
  total: number;
  used: number;
  available: number;
  percent: number;
};

export type DiskStats = {
  mount: string;
  device: string;
  fsType: string;
  total: number;
  used: number;
  free: number;
  percent: number;
};

export type AppStats = {
  collectedAt: string;
  runtime: {
    goVersion: string;
    goos: string;
    goarch: string;
    numCpu: number;
    gomaxprocs: number;
    goroutines: number;
    heapAlloc: number;
    heapInuse: number;
    heapSys: number;
    heapObjects: number;
    stackInuse: number;
    sys: number;
    nextGc: number;
    numGc: number;
    pauseTotalMs: number;
    lastPauseMs: number;
    lastGcAt: string | null;
  };
  process: {
    instanceId: string;
    version: string;
    pid: number;
    startedAt: string;
    uptimeSeconds: number;
  };
  database: {
    maxOpenConnections: number;
    openConnections: number;
    inUse: number;
    idle: number;
    waitCount: number;
    waitDurationMs: number;
    maxIdleClosed: number;
    maxIdleTimeClosed: number;
    maxLifetimeClosed: number;
  };
  redis: {
    available: boolean;
    version: string;
    mode: string;
    usedMemoryHuman: string;
    usedMemoryPeakHuman: string;
    maxMemoryHuman: string;
    connectedClients: number;
    uptimeSeconds: number;
  };
};

export const getServerStats = () => http.get<ServerStats>("/monitor/server");

export const getAppStats = () => http.get<AppStats>("/monitor/app");
