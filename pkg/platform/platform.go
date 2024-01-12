package platform

type ResourceUsageReport struct {
	TimeUser        int64
	TimeSystem      int64
	MinorFault      int64
	MajorFault      int64
	MaxResidentSize int64
}
