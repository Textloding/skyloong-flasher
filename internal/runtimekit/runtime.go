package runtimekit

import (
	"os"
	"os/exec"
	"path/filepath"
)

const (
	KindMissing      = "missing"
	KindExecutable   = "executable"
	KindPythonScript = "python-script"
)

type Status struct {
	Available    bool   `json:"available"`
	CanBuild     bool   `json:"canBuild"`
	Kind         string `json:"kind"`
	ToolPath     string `json:"toolPath"`
	PythonPath   string `json:"pythonPath"`
	IDFPyPath    string `json:"idfPyPath"`
	ExportScript string `json:"exportScript"`
	Message      string `json:"message"`
}

func Detect() Status {
	status := Status{Kind: KindMissing, Message: "未检测到刷机运行时，后续可由工具自动下载或使用已安装 ESP-IDF"}
	if path, err := exec.LookPath("esptool.exe"); err == nil {
		status.Available = true
		status.Kind = KindExecutable
		status.ToolPath = path
		status.Message = "已检测到 esptool.exe"
	}
	if path, err := exec.LookPath("esptool.py"); err == nil {
		if python, ok := findPython(); ok {
			status.Available = true
			status.Kind = KindPythonScript
			status.ToolPath = path
			status.PythonPath = python
			status.Message = "已检测到 esptool.py"
		}
	}
	idf := detectIDF()
	if idf.CanBuild {
		status.CanBuild = true
		status.IDFPyPath = idf.IDFPyPath
		status.ExportScript = idf.ExportScript
		if !status.Available && idf.Available {
			status.Available = true
			status.Kind = idf.Kind
			status.ToolPath = idf.ToolPath
			status.PythonPath = idf.PythonPath
			status.Message = idf.Message
		}
	}
	return status
}

func detectIDF() Status {
	candidates := []string{}
	if idf := os.Getenv("IDF_PATH"); idf != "" {
		candidates = append(candidates, filepath.Join(idf, "components", "esptool_py", "esptool", "esptool.py"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "esp", "esp-idf", "components", "esptool_py", "esptool", "esptool.py"))
	}
	status := Status{}
	if idfpy, err := exec.LookPath("idf.py"); err == nil {
		status.CanBuild = true
		status.IDFPyPath = idfpy
	}
	for _, export := range exportCandidates() {
		if _, err := os.Stat(export); err == nil {
			status.CanBuild = true
			status.ExportScript = export
			break
		}
	}
	for _, tool := range candidates {
		if _, err := os.Stat(tool); err == nil {
			if python, ok := findPython(); ok {
				status.Available = true
				status.Kind = KindPythonScript
				status.ToolPath = tool
				status.PythonPath = python
				status.Message = "已检测到本机 ESP-IDF esptool"
				return status
			}
		}
	}
	return status
}

func findPython() (string, bool) {
	for _, name := range []string{"python.exe", "python3.exe", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, true
		}
	}
	return "", false
}

func exportCandidates() []string {
	candidates := []string{}
	if idf := os.Getenv("IDF_PATH"); idf != "" {
		candidates = append(candidates, filepath.Join(idf, "export.ps1"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "esp", "esp-idf", "export.ps1"))
	}
	return candidates
}
