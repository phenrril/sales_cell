package domain

type Material string

const (
	MaterialPLA  Material = "PLA"
	MaterialPETG Material = "PETG"
	MaterialTPU  Material = "TPU"
)

type MaterialConf struct {
	Material   Material
	CoefPerCM3 float64
	CoefPerMin float64
}
