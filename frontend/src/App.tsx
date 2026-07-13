import { useEffect, useMemo, useRef, useState } from "react";

type FlashFile = { offset: string; path: string; size: number };
type Analysis = {
  sourcePath: string;
  root: string;
  kind: string;
  projectName: string;
  canFlash: boolean;
  needsBuild: boolean;
  chip: string;
  hardwareVersion: string;
  display: string;
  minimumFlashBytes: number;
  minimumPsramBytes: number;
  psramMode: string;
  writeFlashArgs: string[];
  before: string;
  after: string;
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
  gitPath: string;
  message: string;
};
type Device = { name: string; port: string; pnpDeviceId: string; mode: string; canFlash: boolean; score: number; hint: string };
type AnalyzeResponse = { analysis: Analysis; runtime: RuntimeStatus; devices: Device[] };
type TaskProgress = { stage: string; percent: number; message: string };
type CompatibilityOverall = "compatible" | "risk" | "unknown" | "error";
type CheckStatus = "pass" | "warning" | "unknown" | "error";
type TriState = "unknown" | "enabled" | "disabled";
type FirmwareRequirements = {
  chip: string;
  hardwareVersion: string;
  display: string;
  minimumFlashBytes: number;
  requiredPsramBytes: number;
  requiresOctalPsram: boolean;
  flashFiles: { offset: number; size: number; path: string }[];
  partitions: { type: number; subtype: number; offset: number; size: number; label: string; flags: number }[];
};
type DeviceCapabilities = {
  chip: string;
  revision: string;
  flashBytes: number;
  flashDescription: string;
  psramBytes: number;
  psramDescription: string;
  psramMode: string;
  psramKnown: boolean;
  secureBoot: TriState;
  flashEncryption: TriState;
  rawOutput: string[];
};
type CheckResult = {
  code: string;
  status: CheckStatus;
  title: string;
  summary: string;
  resolution: string;
  steps: string[];
  technical: string;
};
type Report = {
  overall: CompatibilityOverall;
  firmware: FirmwareRequirements;
  device: DeviceCapabilities;
  checks: CheckResult[];
  rawLog: string[];
};

declare global {
  interface Window {
    go?: { main?: { App?: Record<string, (...args: any[]) => Promise<any>> } };
    runtime?: { EventsOn?: (name: string, cb: (payload: any) => void) => () => void };
  }
}

const steps = ["选择固件", "准备固件", "连接屏幕", "兼容性", "刷机"];

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
  const [logFilePath, setLogFilePath] = useState("");
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState("");
  const [taskProgress, setTaskProgress] = useState<TaskProgress>({ stage: "待命", percent: 0, message: "选择固件来源后开始。" });
  const [error, setError] = useState("");
  const [preflight, setPreflight] = useState<Report | null>(null);
  const [preflightBusy, setPreflightBusy] = useState(false);
  const [preflightError, setPreflightError] = useState("");
  const [preflightDetectedAt, setPreflightDetectedAt] = useState<Date | null>(null);
  const logRef = useRef<HTMLPreElement | null>(null);
  const preflightRequestRef = useRef(0);
  const preflightInputVersionRef = useRef(0);
  const preflightKeyRef = useRef("");
  const preflightInputsRef = useRef<{ analysis: Analysis | null; selectedPort: string }>({ analysis: null, selectedPort: "" });

  useEffect(() => {
    const offLog = window.runtime?.EventsOn?.("flash:log", (payload) => {
      setLogs((items) => [...items, payload.line]);
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
    void loadLogHistory();
    void refreshRuntime();
    void scanDevices(false);
    return () => {
      offLog?.();
      offDownload?.();
      offTask?.();
    };
  }, []);

  useEffect(() => {
    const node = logRef.current;
    if (node) node.scrollTop = node.scrollHeight;
  }, [logs]);

  useEffect(() => {
    const previous = preflightInputsRef.current;
    if (previous.analysis === analysis && previous.selectedPort === selectedPort) return;

    preflightInputsRef.current = { analysis, selectedPort };
    preflightInputVersionRef.current += 1;
    preflightRequestRef.current += 1;
    preflightKeyRef.current = "";
    setPreflight(null);
    setPreflightBusy(false);
    setPreflightError("");
    setPreflightDetectedAt(null);

    if (analysis?.canFlash && selectedPort) {
      void detectCompatibility(false, preflightInputVersionRef.current);
    }
  }, [analysis, selectedPort]);

  const activeStep = useMemo(() => {
    if (!analysis) return 0;
    if (analysis.needsBuild || !analysis.canFlash) return 1;
    if (!selectedPort) return 2;
    if (!preflight && !preflightError) return 3;
    return 4;
  }, [analysis, selectedPort, preflight, preflightError]);

  const bestDevice = devices.find((d) => d.canFlash) ?? devices[0];
  const flashButtonText = busy ? "任务执行中..." : analysis?.needsBuild ? "请先构建固件" : "开始刷机";

  async function refreshRuntime() {
    setTaskProgress({ stage: "检查环境", percent: 20, message: "正在检测刷机运行时。" });
    const next = await call<RuntimeStatus>("CheckRuntime");
    setRuntime(next);
    setTaskProgress({ stage: "检查环境", percent: 100, message: next.message || "环境检查完成。" });
  }

  async function loadLogHistory() {
    try {
      const [history, path] = await Promise.all([
        call<string[]>("GetLogHistory"),
        call<string>("GetLogFilePath"),
      ]);
      if (history.length > 0) {
        setLogs((items) => {
          const onlyPlaceholder = items.length === 1 && items[0] === "等待选择固件包。";
          return onlyPlaceholder ? history : [...history, ...items.filter((item) => item !== "等待选择固件包。")];
        });
      }
      if (path) setLogFilePath(path);
    } catch {
      // 浏览器预览模式下没有后端日志历史，保留本地占位日志即可。
    }
  }

  async function scanDevices(showProgress = true) {
    if (showProgress) {
      setTaskProgress({ stage: "扫描设备", percent: 20, message: "正在扫描 USB 和串口设备。" });
    }
    const next = await call<Device[]>("ScanDevices");
    setDevices(next);
    const flashDevice = next.find((d) => d.canFlash && d.port);
    if (flashDevice) setSelectedPort(flashDevice.port);
    if (showProgress) {
      setTaskProgress({ stage: "扫描设备", percent: 100, message: `扫描完成，发现 ${next.length} 个候选设备。` });
    }
  }

  async function detectCompatibility(force = false, inputVersion = preflightInputVersionRef.current) {
    const requestAnalysis = analysis;
    const requestPort = selectedPort;
    if (!requestAnalysis?.canFlash || !requestPort) return;

    const requestKey = `${inputVersion}:${requestPort}`;
    if (!force && preflightKeyRef.current === requestKey) return;

    const requestId = ++preflightRequestRef.current;
    preflightKeyRef.current = requestKey;
    setPreflightBusy(true);
    setPreflightError("");

    try {
      const result = await call<Report>("DetectCompatibility", { port: requestPort, baud: 460800 });
      if (!isCurrentPreflightRequest(requestId, inputVersion, requestAnalysis, requestPort)) return;
      setPreflight(result);
      setPreflightDetectedAt(new Date());
    } catch (err) {
      if (!isCurrentPreflightRequest(requestId, inputVersion, requestAnalysis, requestPort)) return;
      setPreflight(null);
      setPreflightError(userFacingError(err));
      setPreflightDetectedAt(new Date());
    } finally {
      if (isCurrentPreflightRequest(requestId, inputVersion, requestAnalysis, requestPort)) {
        setPreflightBusy(false);
      }
    }
  }

  function isCurrentPreflightRequest(requestId: number, inputVersion: number, requestAnalysis: Analysis, requestPort: string) {
    const currentInputs = preflightInputsRef.current;
    return (
      preflightRequestRef.current === requestId &&
      preflightInputVersionRef.current === inputVersion &&
      currentInputs.analysis === requestAnalysis &&
      currentInputs.selectedPort === requestPort
    );
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
      setError(userFacingError(err));
      setTaskProgress({ stage: "任务失败", percent: 100, message: "请查看错误提示和高级日志。" });
      setLogs((items) => [...items, `解析失败：${String(err)}`]);
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
      setError(userFacingError(err));
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
      setError(userFacingError(err));
      setLogs((items) => [...items, `刷机失败：${String(err)}`]);
      setTaskProgress({ stage: "刷机失败", percent: 100, message: "连接或刷写失败，请查看高级日志。" });
    } finally {
      setBusy(false);
    }
  }

  async function copyLogs() {
    const text = logs.join("\n");
    try {
      await navigator.clipboard?.writeText(text);
      setLogs((items) => [...items, "日志已复制到剪贴板。"]);
    } catch {
      setLogs((items) => [...items, "日志复制失败，请直接选中高级日志内容复制。"]);
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
              <h2>准备固件</h2>
              <p>{analysis ? analysis.projectName : "等待解析固件包。"}</p>
            </div>
          </div>
          <InfoGrid
            rows={[
              ["类型", analysis?.kind ?? "未选择"],
              ["芯片", analysis?.chip ?? "esp32s3"],
              ["刷机文件", analysis ? `${analysis.flashFiles?.length ?? 0} 个` : "0 个"],
              ["当前步骤", analysis?.needsBuild ? "准备环境并构建" : analysis?.canFlash ? "等待连接屏幕" : "等待解析"],
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
          <button className="secondary full" onClick={() => scanDevices(true)}>重新扫描设备</button>
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

        <CompatibilitySection
          analysis={analysis}
          preflight={preflight}
          busy={preflightBusy}
          error={preflightError}
          detectedAt={preflightDetectedAt}
          hasPort={Boolean(selectedPort)}
          onRetry={() => void detectCompatibility(true)}
        />

        <div className="glass flash-card">
          <div className="section-title">
            <span>05</span>
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
            <summary>高级日志（完整记录）</summary>
            <div className="log-toolbar">
              <span>{logFilePath ? `日志文件：${logFilePath}` : "当前会话日志会从开头保留到最后。"}</span>
              <button className="secondary mini" type="button" onClick={copyLogs}>复制日志</button>
            </div>
            <pre ref={logRef}>{logs.join("\n")}</pre>
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

function CompatibilitySection({
  analysis,
  preflight,
  busy,
  error,
  detectedAt,
  hasPort,
  onRetry,
}: {
  analysis: Analysis | null;
  preflight: Report | null;
  busy: boolean;
  error: string;
  detectedAt: Date | null;
  hasPort: boolean;
  onRetry: () => void;
}) {
  const overall = preflight?.overall ?? (error ? "error" : undefined);
  const overallLabel: Record<CompatibilityOverall, string> = {
    compatible: "兼容",
    risk: "检测到风险",
    unknown: "部分信息未知",
    error: "检测失败",
  };

  return (
    <section className="compatibility-section" aria-labelledby="compatibility-title" aria-busy={busy}>
      <div className="compatibility-heading">
        <div>
          <p className="section-kicker">04</p>
          <h2 id="compatibility-title">设备兼容性检测</h2>
          <p>对比当前固件与已选设备的刷写条件。</p>
        </div>
        <div className="compatibility-actions">
          {overall && <strong className={`compatibility-overall ${overall}`}>{overallLabel[overall]}</strong>}
          {detectedAt && <time dateTime={detectedAt.toISOString()}>检测时间：{detectedAt.toLocaleString()}</time>}
          <button type="button" className="secondary" onClick={onRetry} disabled={!analysis?.canFlash || !hasPort || busy}>
            重新检测
          </button>
        </div>
      </div>

      {busy && <p className="compatibility-loading" role="status" aria-live="polite">正在读取设备信息并比对兼容性...</p>}
      {error && <div className="error compatibility-error" role="alert">{error}</div>}

      {preflight && (
        <>
          <dl className="compatibility-facts">
            <div>
              <dt>芯片</dt>
              <dd><span>固件：{preflight.firmware.chip || "未知"}</span><span>设备：{preflight.device.chip || "未知"}</span></dd>
            </div>
            <div>
              <dt>Flash</dt>
              <dd><span>固件：至少 {formatSize(preflight.firmware.minimumFlashBytes)}</span><span>设备：{preflight.device.flashDescription || formatSize(preflight.device.flashBytes)}</span></dd>
            </div>
            <div>
              <dt>PSRAM</dt>
              <dd><span>固件：{preflight.firmware.requiredPsramBytes ? `${formatSize(preflight.firmware.requiredPsramBytes)}${preflight.firmware.requiresOctalPsram ? "，octal" : ""}` : "未要求"}</span><span>设备：{preflight.device.psramKnown ? `${preflight.device.psramDescription || formatSize(preflight.device.psramBytes)}${preflight.device.psramMode ? `，${preflight.device.psramMode}` : ""}` : "未知"}</span></dd>
            </div>
            <div>
              <dt>安全状态</dt>
              <dd><span>固件：通用固件</span><span>设备：安全启动 {triStateLabel(preflight.device.secureBoot)}；Flash 加密 {triStateLabel(preflight.device.flashEncryption)}</span></dd>
            </div>
            <div>
              <dt>硬件版本</dt>
              <dd><span>固件：{preflight.firmware.hardwareVersion || "未声明"}{preflight.firmware.display ? `（${preflight.firmware.display}）` : ""}</span><span>设备：通用探测未报告主板版本</span></dd>
            </div>
          </dl>

          <ol className="compatibility-checks">
            {preflight.checks.map((check, index) => (
              <li key={`${check.code}-${index}`} className={`compatibility-check ${check.status}`}>
                <div className="check-heading">
                  <strong>{check.title}</strong>
                  <span>{checkStatusLabel(check.status)}</span>
                </div>
                <p>{check.summary}</p>
                {check.status !== "pass" && (check.resolution || check.steps.length > 0) && (
                  <div className="check-resolution">
                    {check.resolution && <p><strong>解决建议：</strong>{check.resolution}</p>}
                    {check.steps.length > 0 && <ol>{check.steps.map((step, stepIndex) => <li key={`${check.code}-step-${stepIndex}`}>{step}</li>)}</ol>}
                  </div>
                )}
                {check.technical && <details><summary>技术详情</summary><pre>{check.technical}</pre></details>}
              </li>
            ))}
          </ol>

          {preflight.rawLog.length > 0 && <details className="compatibility-raw-log"><summary>原始检测日志</summary><pre>{preflight.rawLog.join("\n")}</pre></details>}
        </>
      )}

      {!preflight && !busy && !error && <p className="compatibility-empty">{analysis?.canFlash && hasPort ? "等待兼容性检测结果。" : "选择可刷写固件和设备串口后自动开始检测。"}</p>}
    </section>
  );
}

function checkStatusLabel(status: CheckStatus) {
  return { pass: "通过", warning: "风险", unknown: "未知", error: "失败" }[status];
}

function triStateLabel(state: TriState) {
  return { enabled: "已启用", disabled: "未启用", unknown: "未知" }[state];
}

function userFacingError(err: unknown) {
  const raw = String(err ?? "").replace(/^Error:\s*/i, "");
  const lower = raw.toLowerCase();
  if (lower.includes("git was not found") || lower.includes("git not found") || lower.includes("failed to get git path")) {
    return "工具没有成功准备 Git 运行时。请重新点击“准备环境并构建”，工具会自动下载便携 Git；如果仍失败，请使用离线完整版。";
  }
  if (
    raw.includes("便携 Git 下载或解压失败") ||
    raw.includes("EIM CLI 下载失败") ||
    lower.includes("wsarecv") ||
    lower.includes("timed out") ||
    lower.includes("timeout") ||
    lower.includes("connection attempt failed")
  ) {
    return "构建环境下载失败。请稍后重试，或使用离线完整版；工具会优先尝试国内镜像，详细失败原因在高级日志里。";
  }
  if (raw.includes("ESP-IDF 自动安装失败")) {
    return "ESP-IDF 构建环境自动准备失败。请先重试一次；如果网络较慢，等待界面进度继续变化，不需要打开命令行。详细原因在高级日志里。";
  }
  if (raw.includes("组件缓存路径过长") || raw.includes("ComponentManager") || raw.includes("FileNotFoundError")) {
    return "ESP-IDF 组件缓存路径过深或缓存不完整。工具已改用更短的组件缓存目录，请直接重新点击“准备环境并构建”。";
  }
  if (raw.includes("源码构建失败")) {
    return "源码构建失败。请查看高级日志里最早出现的 CMake/idf.py 错误；工具会保留完整日志，必要时可复制出来排查。";
  }
  return raw || "任务失败，请查看高级日志。";
}

async function mockCall(name: string, ...args: any[]): Promise<any> {
  await new Promise((resolve) => setTimeout(resolve, 240));
  if (name === "CheckRuntime") {
    return { available: false, canBuild: false, kind: "missing", toolPath: "", pythonPath: "", idfPyPath: "", exportScript: "", gitPath: "", message: "浏览器预览模式：未连接 Go 后端" };
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
        hardwareVersion: "SCM_V4.0",
        display: "320x240-st7789-8bit-parallel",
        minimumFlashBytes: 16777216,
        minimumPsramBytes: 8388608,
        psramMode: "octal",
        writeFlashArgs: ["--flash_mode", "dio", "--flash_size", "detect", "--flash_freq", "80m"],
        messages: ["浏览器预览：已模拟解析固件包。"],
        flashFiles: [
          { offset: "0x0", path: "bootloader/bootloader.bin", size: 20880 },
          { offset: "0x8000", path: "partition_table/partition-table.bin", size: 3072 },
          { offset: "0x10000", path: "ota_data_initial.bin", size: 8192 },
          { offset: "0x20000", path: "GK87-Screen.bin", size: 4994032 },
        ],
      },
      runtime: { available: false, canBuild: false, kind: "missing", toolPath: "", pythonPath: "", idfPyPath: "", exportScript: "", gitPath: "", message: "浏览器预览模式" },
      devices: await mockCall("ScanDevices"),
    };
  }
  if (name === "GetLogHistory") return ["浏览器预览模式：等待连接桌面后端。"];
  if (name === "GetLogFilePath") return "";
  if (name === "BuildSourcePackage") {
    return mockCall("AnalyzeLocalZip", "preview.zip");
  }
  if (name === "DetectCompatibility") {
    return {
      overall: "unknown",
      firmware: {
        chip: "esp32s3", hardwareVersion: "SCM_V4.0", display: "320x240-st7789-8bit-parallel", minimumFlashBytes: 16777216,
        requiredPsramBytes: 8388608, requiresOctalPsram: true,
        flashFiles: [{ offset: 0, size: 20880, path: "bootloader/bootloader.bin" }, { offset: 131072, size: 4994032, path: "GK87-Screen.bin" }], partitions: [],
      },
      device: {
        chip: "ESP32-S3", revision: "v0.2", flashBytes: 16777216, flashDescription: "16.0 MB", psramBytes: 8388608, psramDescription: "8.0 MB",
        psramMode: "octal", psramKnown: true, secureBoot: "disabled", flashEncryption: "disabled",
        rawOutput: ["Chip is ESP32-S3 (revision v0.2)", "Detected 16MB flash and 8MB octal PSRAM"],
      },
      checks: [
        { code: "chip_compatible", status: "pass", title: "芯片型号匹配", summary: "固件与设备均为 ESP32-S3", resolution: "", steps: [], technical: "" },
        { code: "flash_compatible", status: "pass", title: "Flash 容量满足要求", summary: "设备 16.0 MB，固件至少需要 16.0 MB", resolution: "", steps: [], technical: "" },
        { code: "psram_compatible", status: "pass", title: "PSRAM 容量满足要求", summary: "设备 8.0 MB，固件至少需要 8.0 MB", resolution: "", steps: [], technical: "" },
        { code: "hardware_unknown", status: "unknown", title: "无法自动确认主板硬件版本", summary: "通用 USB 探测不能区分主板 V3/V4，当前固件面向 SCM_V4.0。", resolution: "V3/V4 显示接线不同，反复刷机不能修复接线差异；请核对 PCB 丝印并选择 V4 包。", steps: ["查看显示板或主板上的 PCB 丝印，确认实际硬件版本。", "确认丝印为 V4 后，选择标注 SCM_V4.0 的 V4 包。", "若不是 V4，不要继续反复刷机；改用与 PCB 版本匹配的固件包。"], technical: "required hardware=SCM_V4.0; generic USB probe has no board revision" },
      ],
      rawLog: ["probe: COM3 @ 460800", "Chip is ESP32-S3 (revision v0.2)"],
    } satisfies Report;
  }
  if (name === "StartFlash") return undefined;
  return undefined;
}

export default App;
