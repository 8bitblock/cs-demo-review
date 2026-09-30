package model

type Demo struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	Engine          string   `json:"engine"`
	Map             string   `json:"map"`
	Date            string   `json:"date"`
	Duration        float64  `json:"duration"`
	TickRate        float64  `json:"tickRate"`
	TotalTicks      int      `json:"totalTicks"`
	Status          string   `json:"status"`
	PlayerCount     int      `json:"playerCount"`
	RoundCount      int      `json:"roundCount"`
	FileSize        int64    `json:"fileSize"`
	Hash            string   `json:"hash"`
	ParserVersion   string   `json:"parserVersion"`
	AnalysisVersion string   `json:"analysisVersion"`
	RecordingType   string   `json:"recordingType"`
	MapVersion      string   `json:"mapVersion"`
	Warnings        []string `json:"warnings"`
}
type Capability struct {
	Signal          string `json:"signal"`
	Status          string `json:"status"`
	Reason          string `json:"reason"`
	Samples         int    `json:"samples"`
	MeasuredSamples int    `json:"measuredSamples"`
	Basis           string `json:"basis,omitempty"`
}
type Player struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Team          string         `json:"team"`
	Kills         int            `json:"kills"`
	Deaths        int            `json:"deaths"`
	Assists       int            `json:"assists"`
	Headshots     int            `json:"headshots"`
	Damage        int            `json:"damage"`
	Verdict       string         `json:"verdict"`
	EvidenceCount int            `json:"evidenceCount"`
	Capabilities  []Capability   `json:"capabilities"`
	Coverage      string         `json:"coverage"`
	Review        *ReviewSummary `json:"review,omitempty"`
}

// ReviewSummary records neutral measurements even when a cheating assessment
// cannot be supported. Clips are navigation aids, never additional findings.
type ReviewSummary struct {
	Summary          string            `json:"summary"`
	TotalShots       int               `json:"totalShots"`
	SampledShots     int               `json:"sampledShots"`
	EligibleAimShots int               `json:"eligibleAimShots"`
	CoveredRounds    int               `json:"coveredRounds"`
	MedianSampleMS   *float64          `json:"medianSampleMs"`
	Metrics          []ReviewMetric    `json:"metrics"`
	Exclusions       []ReviewExclusion `json:"exclusions"`
	Clips            []ReviewClip      `json:"clips"`
}
type ReviewMetric struct {
	Signal     string  `json:"signal,omitempty"`
	Provenance string  `json:"provenance,omitempty"`
	Label      string  `json:"label"`
	Value      float64 `json:"value"`
	Unit       string  `json:"unit"`
	Samples    int     `json:"samples"`
}
type ReviewExclusion struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}
type ReviewClip struct {
	Signal       string        `json:"signal,omitempty"`
	Provenance   string        `json:"provenance,omitempty"`
	ID           string        `json:"id"`
	Round        int           `json:"round"`
	Tick         int           `json:"tick"`
	EndTick      int           `json:"endTick"`
	Time         float64       `json:"time"`
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Measurements []Measurement `json:"measurements"`
	Limitations  []string      `json:"limitations"`
}
type Round struct {
	Number    int    `json:"number"`
	StartTick int    `json:"startTick"`
	EndTick   int    `json:"endTick"`
	Winner    string `json:"winner"`
	Reason    string `json:"reason"`
}
type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}
type Sample struct {
	SpottedBy    []string `json:"spottedBy,omitempty"`
	SpottedKnown bool     `json:"spottedKnown,omitempty"`
	Tick         int      `json:"tick"`
	Time         float64  `json:"time"`
	PlayerID     string   `json:"playerId"`
	X            float64  `json:"x"`
	Y            float64  `json:"y"`
	Z            float64  `json:"z"`
	Yaw          float64  `json:"yaw"`
	Pitch        float64  `json:"pitch"`
	Health       int      `json:"health"`
	Armor        int      `json:"armor"`
	Team         string   `json:"team"`
	Alive        bool     `json:"alive"`
	Weapon       string   `json:"weapon"`
	Scoped       bool     `json:"scoped"`
	Flashed      bool     `json:"flashed"`
	Crouching    bool     `json:"crouching"`
	Velocity     float64  `json:"velocity"`
	Eye          *Vec3    `json:"eye,omitempty"`
	RecoilIndex  *float64 `json:"recoilIndex,omitempty"`
	AimPunch     *Vec3    `json:"aimPunch,omitempty"`
}
type GameEvent struct {
	ID       string   `json:"id"`
	Tick     int      `json:"tick"`
	Time     float64  `json:"time"`
	Round    int      `json:"round"`
	Kind     string   `json:"kind"`
	PlayerID string   `json:"playerId,omitempty"`
	TargetID string   `json:"targetId,omitempty"`
	Weapon   string   `json:"weapon,omitempty"`
	X        *float64 `json:"x,omitempty"`
	Y        *float64 `json:"y,omitempty"`
	Z        *float64 `json:"z,omitempty"`
	Text     string   `json:"text"`
	Headshot bool     `json:"headshot,omitempty"`
}
type Shot struct {
	ID              string   `json:"id"`
	Tick            int      `json:"tick"`
	Time            float64  `json:"time"`
	Round           int      `json:"round"`
	PlayerID        string   `json:"playerId"`
	Weapon          string   `json:"weapon"`
	Origin          *Vec3    `json:"origin,omitempty"`
	ViewAngles      *Vec3    `json:"viewAngles,omitempty"`
	NativeAngles    *Vec3    `json:"nativeAngles,omitempty"`
	AimPunch        *Vec3    `json:"aimPunch,omitempty"`
	AimPunchScale   *float64 `json:"aimPunchScale,omitempty"`
	RecoilIndex     *float64 `json:"recoilIndex,omitempty"`
	Spread          *float64 `json:"spread,omitempty"`
	Inaccuracy      *float64 `json:"inaccuracy,omitempty"`
	Impacts         []Vec3   `json:"impacts"`
	TimingPrecision float64  `json:"timingPrecision"`
	Provenance      string   `json:"provenance"`
	Ambiguous       bool     `json:"ambiguous"`
}
type Measurement struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}
type Finding struct {
	ID           string        `json:"id"`
	DemoID       string        `json:"demoId"`
	PlayerID     string        `json:"playerId"`
	Round        int           `json:"round"`
	Tick         int           `json:"tick"`
	EndTick      int           `json:"endTick"`
	Time         float64       `json:"time"`
	Signal       string        `json:"signal"`
	Severity     string        `json:"severity"`
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Measurements []Measurement `json:"measurements"`
	Alternatives []string      `json:"alternatives"`
	Limitations  []string      `json:"limitations"`
	EpisodeID    string        `json:"episodeId"`
}
type ReviewNote struct {
	ID        string `json:"id"`
	DemoID    string `json:"demoId"`
	Tick      int    `json:"tick"`
	PlayerID  string `json:"playerId"`
	Text      string `json:"text"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"createdAt"`
}
type MatchDetail struct {
	Demo     Demo         `json:"demo"`
	Players  []Player     `json:"players"`
	Rounds   []Round      `json:"rounds"`
	Findings []Finding    `json:"findings"`
	Events   []GameEvent  `json:"events"`
	Notes    []ReviewNote `json:"notes"`
}
type ReplayWindow struct {
	Samples  []Sample    `json:"samples"`
	Events   []GameEvent `json:"events"`
	Shots    []Shot      `json:"shots"`
	FromTick int         `json:"fromTick"`
	ToTick   int         `json:"toTick"`
}
type ImportProgress struct {
	JobID    string  `json:"jobId"`
	Path     string  `json:"path"`
	Name     string  `json:"name"`
	Stage    string  `json:"stage"`
	Progress float64 `json:"progress"`
	Message  string  `json:"message"`
	DemoID   string  `json:"demoId,omitempty"`
}
type MapFloor struct {
	Name  string  `json:"name"`
	MinZ  float64 `json:"minZ"`
	MaxZ  float64 `json:"maxZ"`
	Image string  `json:"image,omitempty"`
}
type MapAsset struct {
	ID           string     `json:"id"`
	Map          string     `json:"map"`
	Engine       string     `json:"engine"`
	Version      string     `json:"version"`
	Source       string     `json:"source"`
	Verified     bool       `json:"verified"`
	PosX         float64    `json:"posX"`
	PosY         float64    `json:"posY"`
	Scale        float64    `json:"scale"`
	Rotate       float64    `json:"rotate"`
	Floors       []MapFloor `json:"floors"`
	GeometryPath string     `json:"geometryPath,omitempty"`
	Image        string     `json:"image,omitempty"`
	Warnings     []string   `json:"warnings"`
}
type AppSettings struct {
	CS2Path           string `json:"cs2Path"`
	CSGOPath          string `json:"csgoPath"`
	SteamPath         string `json:"steamPath"`
	Source2ViewerPath string `json:"source2ViewerPath"`
	LibraryPath       string `json:"libraryPath"`
}
