package tipos

type TipoCompresionBloque string

const (
	Ninguna TipoCompresionBloque = "Ninguna"
	LZ4     TipoCompresionBloque = "LZ4"
	ZSTD    TipoCompresionBloque = "ZSTD"
	Snappy  TipoCompresionBloque = "Snappy"
	Gzip    TipoCompresionBloque = "Gzip"
)

type TipoCompresionValores string

const (
	SinCompresion TipoCompresionValores = "SinCompresion"
	DeltaDelta    TipoCompresionValores = "DeltaDelta"
	RLE           TipoCompresionValores = "RLE"
	Bits          TipoCompresionValores = "Bits"
)

type TipoDatos string

const (
	TipoNumerico   TipoDatos = "NUMERICO"
	TipoCategorico TipoDatos = "CATEGORICO"
	TipoMixto      TipoDatos = "MIXTO"
)

