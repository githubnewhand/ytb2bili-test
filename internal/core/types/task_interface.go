package types

const TaskProgressReporterKey = "task_progress_reporter"

type ProgressReporter func(percent int, message string)

// Task 接口定义了任务处理器的基本操作
type Task interface {
	Execute(context map[string]interface{}) bool
	GetName() string
	InsertTask() error
	UpdateStatus(status, message string) error
}

func ReportTaskProgress(context map[string]interface{}, percent int, message string) {
	if context == nil {
		return
	}

	reporter, ok := context[TaskProgressReporterKey].(ProgressReporter)
	if !ok || reporter == nil {
		return
	}

	reporter(percent, message)
}
