package printer

type seedPlateLayout struct {
	LeftColXMM    float64
	RightColXMM   float64
	QRLeftMM      float64
	RightMetaText string
	ShareNum      int
	ShareTotal    int
}

const (
	seedQRYBottomMarginMM = 10.0
	seedQRSizeMM          = 32.0
)

func defaultSeedPlateLayout(totalShares int, singlesigVariant bool) seedPlateLayout {
	layout := seedPlateLayout{
		LeftColXMM:  10.0,
		RightColXMM: 49.0,
		QRLeftMM:    49.0,
	}
	if totalShares == 1 || singlesigVariant {
		layout.LeftColXMM = 8.0
		layout.RightColXMM = 47.0
		layout.QRLeftMM = 47.0
	}
	return layout
}

func seedQRYMM(qrSizeMM float64) float64 {
	return plateSizeMM - seedQRYBottomMarginMM - qrSizeMM
}
