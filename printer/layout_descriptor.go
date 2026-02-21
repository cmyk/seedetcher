package printer

type descriptorSingleQRLayoutSpec struct {
	MarginMM    float64
	LineGapMM   float64
	PathGapMM   float64
	QRXMM       float64
	QRYMM       float64
	DefaultSize float64
}

type descriptorDualQRLayoutSpec struct {
	MarginMM       float64
	LineGapMM      float64
	GuideGapMM     float64
	QRTopLimitMM   float64
	QuietModules   int
	OverlapIters   int
	OverlapEpsilon float64
}

var (
	descriptorSingleQRLayout = descriptorSingleQRLayoutSpec{
		MarginMM:    3.0,
		LineGapMM:   4.2,
		PathGapMM:   4.2,
		QRXMM:       12.0,
		QRYMM:       12.0,
		DefaultSize: 80.0,
	}

	descriptorDualQRLayout = descriptorDualQRLayoutSpec{
		MarginMM:       3.0,
		LineGapMM:      4.2,
		GuideGapMM:     1.5,
		QRTopLimitMM:   24.0,
		QuietModules:   4,
		OverlapIters:   8,
		OverlapEpsilon: 0.001,
	}
)
