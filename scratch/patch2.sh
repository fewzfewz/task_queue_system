#!/bin/bash
sed -i '/defer cancel()/a \
\	execCtx, cancelFunc := context.WithCancel(execCtx)\
\	if wp.onRegister != nil {\
\		wp.onRegister(job.ID, cancelFunc)\
\	}\
\	defer func() {\
\		cancelFunc()\
\		if wp.onUnregister != nil {\
\			wp.onUnregister(job.ID)\
\		}\
\	}()' internal/worker/executor/worker_processor.go
