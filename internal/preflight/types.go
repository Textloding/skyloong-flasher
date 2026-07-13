package preflight

const (
	StatusPass    = "pass"
	StatusWarning = "warning"
	StatusUnknown = "unknown"
	StatusError   = "error"
)

type TriState string

const (
	TriStateUnknown  TriState = "unknown"
	TriStateEnabled  TriState = "enabled"
	TriStateDisabled TriState = "disabled"
)

type Partition struct {
	Type    uint8  `json:"type"`
	Subtype uint8  `json:"subtype"`
	Offset  uint32 `json:"offset"`
	Size    uint32 `json:"size"`
	Label   string `json:"label"`
	Flags   uint32 `json:"flags"`
}

type FlashRegion struct {
	Offset uint64 `json:"offset"`
	Size   uint64 `json:"size"`
	Path   string `json:"path"`
}

type FirmwareRequirements struct {
	Chip               string        `json:"chip"`
	HardwareVersion    string        `json:"hardwareVersion"`
	Display            string        `json:"display"`
	MinimumFlashBytes  uint64        `json:"minimumFlashBytes"`
	RequiredPSRAMBytes uint64        `json:"requiredPsramBytes"`
	RequiresOctalPSRAM bool          `json:"requiresOctalPsram"`
	FlashFiles         []FlashRegion `json:"flashFiles"`
	Partitions         []Partition   `json:"partitions"`
}

type DeviceCapabilities struct {
	Chip             string   `json:"chip"`
	Revision         string   `json:"revision"`
	FlashBytes       uint64   `json:"flashBytes"`
	FlashDescription string   `json:"flashDescription"`
	PSRAMBytes       uint64   `json:"psramBytes"`
	PSRAMDescription string   `json:"psramDescription"`
	PSRAMMode        string   `json:"psramMode"`
	PSRAMKnown       bool     `json:"psramKnown"`
	SecureBoot       TriState `json:"secureBoot"`
	FlashEncryption  TriState `json:"flashEncryption"`
	RawOutput        []string `json:"rawOutput"`
}

type DeviceProbe struct {
	Capabilities DeviceCapabilities `json:"capabilities"`
	Checks       []CheckResult      `json:"checks"`
	RawOutput    []string           `json:"rawOutput"`
}

type CheckResult struct {
	Code       string   `json:"code"`
	Status     string   `json:"status"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Resolution string   `json:"resolution"`
	Steps      []string `json:"steps"`
	Technical  string   `json:"technical"`
}

type FirmwareInspection struct {
	Requirements FirmwareRequirements `json:"requirements"`
	Checks       []CheckResult        `json:"checks"`
	RawTechnical []string             `json:"rawTechnical"`
}

type Report struct {
	Overall  string               `json:"overall"`
	Firmware FirmwareRequirements `json:"firmware"`
	Device   DeviceCapabilities   `json:"device"`
	Checks   []CheckResult        `json:"checks"`
	RawLog   []string             `json:"rawLog"`
}
