#!/bin/bash
sed -i 's/	onIdle    func()/	onIdle    func()\n\n\tonRegister   func(string, context.CancelFunc)\n\tonUnregister func(string)/' internal/worker/executor/worker_processor.go
cat << 'HOOKS' >> internal/worker/executor/worker_processor.go

// SetCancelHooks configures callbacks to register and unregister context cancellations.
func (wp *WorkerProcessor) SetCancelHooks(register func(jobID string, cancel context.CancelFunc), unregister func(jobID string)) {
	wp.onRegister = register
	wp.onUnregister = unregister
}
HOOKS
