//go:build linux

package platform

import "syscall"

func ReportUsage() (*ResourceUsageReport, error) {
	data := syscall.Rusage{}
	err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &data)
	if err != nil {
		return nil, err
	}

	return &ResourceUsageReport{
		TimeUser:        (data.Utime.Sec * 1e6) + data.Utime.Usec,
		TimeSystem:      (data.Stime.Sec * 1e6) + data.Stime.Usec,
		MinorFault:      data.Minflt,
		MajorFault:      data.Majflt,
		MaxResidentSize: data.Maxrss,
	}, nil
}
