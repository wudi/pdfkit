package raw

// HintTable represents the parsed content of a linearization hint stream.
type HintTable struct {
	PageOffsets   []PageOffsetHint
	SharedObjects []int64 // Offsets of shared objects groups
}

type PageOffsetHint struct {
	MinObjNum      int
	PageLength     int64
	ContentStream  int64 // Offset
	ContentLength  int64
	SharedObjIndex int
}
