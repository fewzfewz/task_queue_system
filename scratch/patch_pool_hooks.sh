#!/bin/bash
sed -i '/go p.metricsLoop(workerCtx)/a \
\	p.wg.Add(1)\
\	go func() {\
\		defer p.wg.Done()\
\		p.listenForCancellations(workerCtx)\
\	}()' internal/worker/pool/pool.go

sed -i '/w.SetHooks(/a \
\		w.SetCancelHooks(p.RegisterActiveJob, p.UnregisterActiveJob)' internal/worker/pool/pool.go
