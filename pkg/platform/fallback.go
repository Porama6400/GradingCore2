//go:build !linux

package platform

func ReportUsage() (*ResourceUsageReport, error) {
	return nil, nil
}
