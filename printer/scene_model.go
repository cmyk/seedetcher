package printer

type PrimitiveKind string
type FillMode string

const (
	PrimitiveRect   PrimitiveKind = "rect"
	PrimitiveRound  PrimitiveKind = "rounded_rect"
	PrimitiveCircle PrimitiveKind = "circle"
	PrimitiveRing   PrimitiveKind = "ring"
	PrimitiveText   PrimitiveKind = "text"
	PrimitivePath   PrimitiveKind = "path"
	PrimitiveGroup  PrimitiveKind = "group"
)

const (
	FillModeNone   FillMode = "none"
	FillModeHatch  FillMode = "hatch"
	FillModeOffset FillMode = "offset"
	FillModeSpiral FillMode = "spiral"
)

type TextDirection string

const (
	TextDirHorizontal   TextDirection = "horizontal"
	TextDirVerticalUp   TextDirection = "vertical_up"
	TextDirVerticalDown TextDirection = "vertical_down"
)

type TextAnchor string

const (
	TextAnchorBaselineLeft TextAnchor = "baseline_left"
	TextAnchorTopLeft      TextAnchor = "top_left"
	TextAnchorCenter       TextAnchor = "center"
)

type ScenePrimitive struct {
	Kind PrimitiveKind `json:"kind"`

	// Geometry (mm-based scene space)
	XMM         float64 `json:"x_mm,omitempty"`
	YMM         float64 `json:"y_mm,omitempty"`
	WidthMM     float64 `json:"width_mm,omitempty"`
	HeightMM    float64 `json:"height_mm,omitempty"`
	ThicknessMM float64 `json:"thickness_mm,omitempty"`
	CXMM        float64 `json:"cx_mm,omitempty"`
	CYMM        float64 `json:"cy_mm,omitempty"`
	RadiusMM    float64 `json:"radius_mm,omitempty"`
	PathData    string  `json:"path_data,omitempty"`

	// Paint/style
	FillColor   string   `json:"fill_color,omitempty"`
	FillRule    string   `json:"fill_rule,omitempty"`
	FillMode    FillMode `json:"fill_mode,omitempty"`
	StrokeColor string   `json:"stroke_color,omitempty"`
	StrokeMM    float64  `json:"stroke_mm,omitempty"`
	PowerS      int      `json:"power_s,omitempty"`
	FeedMMMin   float64  `json:"feed_mm_min,omitempty"`

	// Text
	Text       string        `json:"text,omitempty"`
	FontFamily string        `json:"font_family,omitempty"`
	FontSizePt float64       `json:"font_size_pt,omitempty"`
	TrackingEM float64       `json:"tracking_em,omitempty"`
	Direction  TextDirection `json:"direction,omitempty"`
	Anchor     TextAnchor    `json:"anchor,omitempty"`

	Children []ScenePrimitive `json:"children,omitempty"`
}

type SceneLayer struct {
	Tag        string           `json:"tag"`
	Visible    bool             `json:"visible"`
	Primitives []ScenePrimitive `json:"primitives"`
}

type PlateScene struct {
	Name             string       `json:"name"`
	WidthMM          float64      `json:"width_mm"`
	HeightMM         float64      `json:"height_mm"`
	AnchorInPlate    string       `json:"anchor_in_plate,omitempty"`
	OffsetInPlateXMM float64      `json:"offset_in_plate_x_mm,omitempty"`
	OffsetInPlateYMM float64      `json:"offset_in_plate_y_mm,omitempty"`
	Layers           []SceneLayer `json:"layers"`
}

type PlateDocument struct {
	Version string       `json:"version"`
	Scenes  []PlateScene `json:"scenes"`
}

// SceneBuilder builds scene-only layout data.
type SceneBuilder interface {
	Build() (*PlateDocument, error)
}

// SceneRenderer renders a scene document to a target output.
type SceneRenderer interface {
	Render(doc *PlateDocument, out string) error
}
