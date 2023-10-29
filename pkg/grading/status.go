package grading

import "fmt"

type StatusCode string

const (
	StatusCompleted StatusCode = "COMPLETED"

	StatusFailCompilation        StatusCode = "FAIL_COMPILATION"
	StatusFailCompilationTimeout StatusCode = "FAIL_COMPILATION_TIMEOUT"
	StatusFailTimeout            StatusCode = "FAIL_TIMEOUT"
	StatusFailTimeoutHard        StatusCode = "FAIL_TIMEOUT_HARD"
	StatusFailMemory             StatusCode = "FAIL_MEMORY"

	StatusSystemFail              StatusCode = "SYSTEM_FAIL"
	StatusSystemFailMissingImage  StatusCode = "SYSTEM_FAIL_MISSING_IMAGE"
	StatusSystemFailFetchFile     StatusCode = "SYSTEM_FAIL_FETCH_FILE"
	StatusSystemFailContainer     StatusCode = "SYSTEM_FAIL_CONTAINER"
	StatusSystemFailContainerPing StatusCode = "SYSTEM_FAIL_CONTAINER_PING"
	StatusSystemFailRetryExceed   StatusCode = "SYSTEM_FAIL_RETRY_EXCEED"

	StatusUnknown StatusCode = "UNKNOWN"
)

type Error struct {
	error
	ErrorCode StatusCode
	Wrap      error
}

func WrapError(errorCode StatusCode, err error) error {
	return &Error{
		ErrorCode: errorCode,
		Wrap:      err,
	}
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s:%s", e.ErrorCode, e.Wrap.Error())
}
