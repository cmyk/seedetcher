package printer

type PrimitiveKind string

const (
	PrimitiveRect   PrimitiveKind = "rect"
	PrimitiveRound  PrimitiveKind = "rounded_rect"
	PrimitiveCircle PrimitiveKind = "circle"
	PrimitiveText   PrimitiveKind = "text"
	PrimitivePath   PrimitiveKind = "path"
	PrimitiveGroup  PrimitiveKind = "group"
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
	XMM      float64 `json:"x_mm,omitempty"`
	YMM      float64 `json:"y_mm,omitempty"`
	WidthMM  float64 `json:"width_mm,omitempty"`
	HeightMM float64 `json:"height_mm,omitempty"`
	CXMM     float64 `json:"cx_mm,omitempty"`
	CYMM     float64 `json:"cy_mm,omitempty"`
	RadiusMM float64 `json:"radius_mm,omitempty"`
	PathData string  `json:"path_data,omitempty"`

	// Paint/style
	FillColor   string  `json:"fill_color,omitempty"`
	StrokeColor string  `json:"stroke_color,omitempty"`
	StrokeMM    float64 `json:"stroke_mm,omitempty"`

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
	Name     string       `json:"name"`
	WidthMM  float64      `json:"width_mm"`
	HeightMM float64      `json:"height_mm"`
	Layers   []SceneLayer `json:"layers"`
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
