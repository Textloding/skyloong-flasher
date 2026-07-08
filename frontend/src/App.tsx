import { useEffect, useMemo, useState } from "react";

type FlashFile = { offset: string; path: string; size: number };
type Analysis = {
  sourcePath: string;
  root: string;
  kind: string;
  projectName: string;
  canFlash: boolean;
  needsBuild: boolean;
  chip: string;
  writeFlashArgs: string[];
  flashFiles: FlashFile[];
  messages: string[];
};
type RuntimeStatus = {
  available: boolean;
  canBuild: boolean;
  kind: string;
  toolPath: string;
  pythonPath: string;
  idfPyPath: string;
  exportScript: string;
  message: string;
};
type Device = { name: string; port: string; pnpDeviceId: string; mode: string; canFlash: boolean; score: number; hint: string };
type AnalyzeResponse = { analysis: Analysis; runtime: RuntimeStatus; devices: Device[] };
type TaskProgress = { stage: string; percent: number; message: string };

declare global {
  interface Window {
    go?: { main?: { App?: Record<string, (...args: any[]) => Promise<any>> } };
    runtime?: { EventsOn?: (name: string, cb: (payload: any) => void) => () => void };
  }
}

const steps = ["选择固件", "解析包", "检查环境", "连接屏幕", "刷机"];

async function call<T>(name: string, ...args: any[]): Promise<T> {
  const fn = window.go?.main?.App?.[name];
  if (!fn) {
    return mockCall(name, ...args) as T;
  }
  return fn(...args);
}

function App() {
  const [sourceMode, setSourceMode] = useState<"zip" | "github">("zip");
  const [zipPath, setZipPath] = useState("");
  const [githubURL, setGithubURL] = useState("https://github.com/Textloding/SKYLOONG");
  const [analysis, setAnalysis] = useState<Analysis | null>(null);
  const [runtime, setRuntime] = useState<RuntimeStatus | null>(null);
  const [devices, setDevices] = useState<Device[]>([]);
  const [selectedPort, setSelectedPort] = useState("");
  const [logs, setLogs] = useState<string[]>(["等待选择固件包。"]);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState("");
  const [taskProgress, setTaskProgress] = useState<TaskProgress>({ stage: "待命", percent: 0, message: "选择固件来源后开始。" });
  const [error, setError] = useState("");

  useEffect(() => {
    void refreshRuntime();
    void scanDevices();
    const offLog = window.runtime?.EventsOn?.("flash:log", (payload) => {
      setLogs((items) => [...items.slice(-160), payload.line]);
    });
    const offDownload = window.runtime?.EventsOn?.("download:progress", (payload) => {
      if (payload.total > 0) {
        setProgress(`下载中 ${Math.round((payload.downloaded / payload.total) * 100)}%`);
      } else {
        setProgress(`已下载 ${formatSize(payload.downloaded)}`);
      }
    });
    const offTask = window.runtime?.EventsOn?.("task:progress", (payload) => {
      setTaskProgress({
        stage: payload.stage ?? "处理中",
        percent: Number(payload.percent ?? 0),
        message: payload.message ?? "",
      });
    });
    return () => {
      offLog?.();
      offDownload?.();
      offTask?.();
    };
  }, []);

  const activeStep = useMemo(() => {
    if (!analysis) return 0;
    if (analysis.needsBuild) return 2;
    if (!runtime?.available) return 2;
    if (!selectedPort) return 3;
    return 4;
  }, [analysis, runtime, selectedPort]);

  const bestDevice = devices.find((d) => d.canFlash) ?? devices[0];
  const flashButtonText = busy ? "任务执行中..." : analysis?.needsBuild ? "请先构建固件" : "开始刷机";

  async function refreshRuntime() {
    setTaskProgress({ stage: "检查环境", percent: 20, message: "正在检测刷机运行时。" });
    const next = await call<RuntimeStatus>("CheckRuntime");
    setRuntime(next);
    setTaskProgress({ stage: "检查环境", percent: 100, message: next.message || "环境检查完成。" });
  }

  async function scanDevices() {
    setTaskProgress({ stage: "扫描设备", percent: 20, message: "正在扫描 USB 和串口设备。" });
    const next = await call<Device[]>("ScanDevices");
    setDevices(next);
    const flashDevice = next.find((d) => d.canFlash && d.port);
    if (flashDevice) setSelectedPort(flashDevice.port);
    setTaskProgress({ stage: "扫描设备", percent: 100, message: `扫描完成，发现 ${next.length} 个候选设备。` });
  }

  async function chooseZip() {
    setError("");
    const path = await call<string>("OpenFirmwareZip");
    if (path) setZipPath(path);
  }

  async function analyze() {
    setBusy(true);
    setError("");
    setProgress("");
    setLogs((items) => [...items, "开始解析固件来源。"]);
    setTaskProgress({
      stage: sourceMode === "zip" ? "解析固件包" : "下载 GitHub 压缩包",
      percent: 5,
      message: sourceMode === "zip" ? "正在读取本地 zip。" : "正在连接 GitHub。",
    });
    try {
      const result =
        sourceMode === "zip"
          ? await call<AnalyzeResponse>("AnalyzeLocalZip", zipPath)
          : await call<AnalyzeResponse>("DownloadAndAnalyzeGithub", { url: githubURL });
      setAnalysis(result.analysis);
      setRuntime(result.runtime);
      setDevices(result.devices);
      const flashDevice = result.devices.find((d) => d.canFlash && d.port);
      if (flashDevice) setSelectedPort(flashDevice.port);
      setLogs((items) => [...items, ...(result.analysis.messages ?? []), "固件来源解析完成。"]);
      setTaskProgress({ stage: "解析固件包", percent: 100, message: "固件来源解析完成。" });
    } catch (err) {
      setError(String(err));
      setTaskProgress({ stage: "任务失败", percent: 100, message: "请查看错误提示和高级日志。" });
    } finally {
      setBusy(false);
    }
  }

  async function buildSource() {
    setBusy(true);
    setError("");
    setLogs((items) => [...items, "开始构建源码包。", "如果本机缺少 ESP-IDF，工具会自动下载并安装到缓存目录。"]);
    setTaskProgress({ stage: "准备构建环境", percent: 5, message: "正在检查 ESP-IDF 构建环境。" });
    try {
      const result = await call<AnalyzeResponse>("BuildSourcePackage");
      setAnalysis(result.analysis);
      setRuntime(result.runtime);
      setDevices(result.devices);
      setTaskProgress({ stage: "构建固件", percent: 100, message: "构建完成，可以开始刷机。" });
      setLogs((items) => [...items, "源码构建完成，刷机产物已准备好。"]);
    } catch (err) {
      setError(String(err));
      setTaskProgress({ stage: "构建失败", percent: 100, message: "请查看错误提示和高级日志。" });
      setLogs((items) => [...items, `构建失败：${String(err)}`]);
    } finally {
      setBusy(false);
    }
  }

  async function flash() {
    setBusy(true);
    setError("");
    setLogs((items) => [...items, "开始刷机任务。", "如果缺少刷机运行时，工具会自动准备。"]);
    setTaskProgress({ stage: "刷机", percent: 5, message: "正在启动刷机进程。" });
    try {
      await call("StartFlash", { port: selectedPort, baud: 460800 });
      setLogs((items) => [...items, "刷机命令执行完成，等待设备重启。"]);
      setTaskProgress({ stage: "刷机", percent: 100, message: "刷机命令执行完成，等待设备重启。" });
    } catch (err) {
      setError(String(err));
      setLogs((items) => [...items, `刷机失败：${String(err)}`]);
      setTaskProgress({ stage: "刷机失败", percent: 100, message: "连接或刷写失败，请查看高级日志。" });
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="app-shell">
      <div className="ambient ambient-a" />
      <div className="ambient ambient-b" />
      <section className="hero-panel">
        <div className="hero-copy">
          <p className="eyebrow">SKYLOONG / GK87 ESP32-S3</p>
          <h1>傻瓜式屏幕刷机工具</h1>
          <p className="subtitle">选择 GitHub 链接或固件 zip，工具会解析包、检查环境、识别屏幕串口，然后引导你完成刷机。</p>
        </div>
        <div className="status-cluster">
          <StatusPill label="运行时" value={runtime?.available ? "已就绪" : "可自动准备"} tone={runtime?.available ? "good" : "warn"} />
          <StatusPill label="设备" value={bestDevice ? bestDevice.mode || "已发现" : "未发现"} tone={bestDevice?.canFlash ? "good" : "warn"} />
          <StatusPill label="固件" value={analysis?.canFlash ? "可刷机" : analysis?.needsBuild ? "需构建" : "待选择"} tone={analysis?.canFlash ? "good" : "neutral"} />
        </div>
      </section>

      <section className="steps glass">
        {steps.map((step, index) => (
          <div className={`step ${index <= activeStep ? "active" : ""}`} key={step}>
            <span>{index + 1}</span>
            <p>{step}</p>
          </div>
        ))}
      </section>

      <section className="glass progress-panel">
        <div>
          <span>{taskProgress.stage}</span>
          <strong>{taskProgress.message}</strong>
        </div>
        <div className="progress-track">
          <i style={{ width: `${Math.max(0, Math.min(100, taskProgress.percent))}%` }} />
        </div>
        <em>{Math.max(0, Math.min(100, Math.round(taskProgress.percent)))}%</em>
      </section>

      <section className="workspace">
        <div className="glass source-card">
          <div className="section-title">
            <span>01</span>
            <div>
              <h2>选择固件来源</h2>
              <p>本地 zip、GitHub 仓库链接、分支 zip 都可以。</p>
            </div>
          </div>
          <div className="segmented">
            <button className={sourceMode === "zip" ? "selected" : ""} onClick={() => setSourceMode("zip")}>本地 ZIP</button>
            <button className={sourceMode === "github" ? "selected" : ""} onClick={() => setSourceMode("github")}>GitHub 链接</button>
          </div>
          {sourceMode === "zip" ? (
            <div className="input-row">
              <input value={zipPath} onChange={(e) => setZipPath(e.target.value)} placeholder="选择或粘贴 zip 文件路径" />
              <button className="secondary" onClick={chooseZip}>浏览</button>
            </div>
          ) : (
            <input value={githubURL} onChange={(e) => setGithubURL(e.target.value)} placeholder="https://github.com/owner/repo 或 /tree/branch" />
          )}
          <button className="primary" disabled={busy || (sourceMode === "zip" ? !zipPath : !githubURL)} onClick={analyze}>
            {busy ? "处理中..." : "解析固件"}
          </button>
          {progress && <p className="hint">{progress}</p>}
          {error && <div className="error">{error}</div>}
        </div>

        <div className="glass info-card">
          <div className="section-title">
            <span>02</span>
            <div>
              <h2>固件状态</h2>
              <p>{analysis ? analysis.projectName : "等待解析固件包。"}</p>
            </div>
          </div>
          <InfoGrid
            rows={[
              ["类型", analysis?.kind ?? "未选择"],
              ["芯片", analysis?.chip ?? "esp32s3"],
              ["刷机文件", analysis ? `${analysis.flashFiles?.length ?? 0} 个` : "0 个"],
              ["构建状态", analysis?.needsBuild ? "需要构建" : analysis?.canFlash ? "可直接刷" : "等待"],
            ]}
          />
          {analysis?.needsBuild && (
            <div className="notice">
              检测到源码包。{runtime?.canBuild ? "已发现 ESP-IDF 构建环境，可以直接构建。" : "点击下方按钮后，工具会自动下载并安装 ESP-IDF，然后继续构建，全程显示进度和日志。"}
            </div>
          )}
          {analysis?.needsBuild && (
            <button className="primary" disabled={busy} onClick={buildSource}>
              {busy ? "任务执行中..." : runtime?.canBuild ? "构建固件" : "准备环境并构建"}
            </button>
          )}
          {analysis?.flashFiles?.length ? (
            <div className="file-list">
              {analysis.flashFiles.map((file) => (
                <div key={file.offset}>
                  <code>{file.offset}</code>
                  <span>{shortPath(file.path)}</span>
                  <em>{formatSize(file.size)}</em>
                </div>
              ))}
            </div>
          ) : null}
        </div>

        <div className="glass device-card">
          <div className="section-title">
            <span>03</span>
            <div>
              <h2>连接屏幕</h2>
              <p>如果只看到键盘运行态，请按住 BOOT/下载键再重新插入或重试扫描。</p>
            </div>
          </div>
          <button className="secondary full" onClick={scanDevices}>重新扫描设备</button>
          <div className="device-list">
            {devices.length === 0 && <div className="empty">还没有检测到可用设备。</div>}
            {devices.map((device) => (
              <button
                key={`${device.port}-${device.pnpDeviceId}-${device.name}`}
                className={`device ${selectedPort === device.port && device.port ? "selected" : ""}`}
                onClick={() => device.port && setSelectedPort(device.port)}
              >
                <strong>{device.port || "无串口"}</strong>
                <span>{device.name}</span>
                <small>{device.hint}</small>
              </button>
            ))}
          </div>
        </div>

        <div className="glass flash-card">
          <div className="section-title">
            <span>04</span>
            <div>
              <h2>开始刷机</h2>
              <p>确认固件可刷、运行时就绪、串口正确后再开始。</p>
            </div>
          </div>
          <InfoGrid
            rows={[
              ["串口", selectedPort || "未选择"],
              ["波特率", "460800"],
              ["运行时", runtime?.message ?? "检测中"],
              ["目标", analysis?.chip ?? "esp32s3"],
            ]}
          />
          <button className="primary danger" disabled={busy || !analysis?.canFlash || !selectedPort} onClick={flash}>
            {flashButtonText}
          </button>
          <details className="logs" open>
            <summary>高级日志</summary>
            <pre>{logs.join("\n")}</pre>
          </details>
        </div>
      </section>
    </main>
  );
}

function StatusPill({ label, value, tone }: { label: string; value: string; tone: "good" | "warn" | "neutral" }) {
  return (
    <div className={`status-pill ${tone}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function InfoGrid({ rows }: { rows: [string, string][] }) {
  return (
    <div className="info-grid">
      {rows.map(([label, value]) => (
        <div key={label}>
          <span>{label}</span>
          <strong>{value}</strong>
        </div>
      ))}
    </div>
  );
}

function formatSize(size: number) {
  if (!size) return "0 B";
  if (size > 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
  if (size > 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${size} B`;
}

function shortPath(path: string) {
  return path.split(/[\\/]/).slice(-2).join("/");
}

async function mockCall(name: string, ...args: any[]): Promise<any> {
  await new Promise((resolve) => setTimeout(resolve, 240));
  if (name === "CheckRuntime") {
    return { available: false, canBuild: false, kind: "missing", toolPath: "", pythonPath: "", idfPyPath: "", exportScript: "", message: "浏览器预览模式：未连接 Go 后端" };
  }
  if (name === "ScanDevices") {
    return [
      { name: "STC USB Keyboard", port: "", pnpDeviceId: "USB\\VID_34BF&PID_FF0E", mode: "runtime", canFlash: false, score: 48, hint: "运行态，需要进入下载模式" },
      { name: "USB JTAG/serial debug unit (COM3)", port: "COM3", pnpDeviceId: "USB\\VID_303A&PID_1001", mode: "flash", canFlash: true, score: 96, hint: "预览设备，可刷机" },
    ];
  }
  if (name === "OpenFirmwareZip") return args[0] ?? "";
  if (name === "AnalyzeLocalZip" || name === "DownloadAndAnalyzeGithub") {
    return {
      analysis: {
        sourcePath: "preview.zip",
        root: "preview/build",
        kind: "flash",
        projectName: "SKYLOONG",
        canFlash: true,
        needsBuild: false,
        chip: "esp32s3",
        writeFlashArgs: ["--flash_mode", "dio", "--flash_size", "detect", "--flash_freq", "80m"],
        messages: ["浏览器预览：已模拟解析固件包。"],
        flashFiles: [
          { offset: "0x0", path: "bootloader/bootloader.bin", size: 20880 },
          { offset: "0x8000", path: "partition_table/partition-table.bin", size: 3072 },
          { offset: "0x10000", path: "ota_data_initial.bin", size: 8192 },
          { offset: "0x20000", path: "GK87-Screen.bin", size: 4994032 },
        ],
      },
      runtime: { available: false, canBuild: false, kind: "missing", toolPath: "", pythonPath: "", idfPyPath: "", exportScript: "", message: "浏览器预览模式" },
      devices: await mockCall("ScanDevices"),
    };
  }
  if (name === "BuildSourcePackage") {
    return mockCall("AnalyzeLocalZip", "preview.zip");
  }
  if (name === "StartFlash") return undefined;
  return undefined;
}

export default App;
