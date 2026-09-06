#!/bin/bash
sed -i 's/		w.SetCancelHooks(p.RegisterActiveJob, p.UnregisterActiveJob)//' internal/worker/pool/pool.go
sed -i '/w.SetHooks(/i \
\		w.SetCancelHooks(p.RegisterActiveJob, p.UnregisterActiveJob)' internal/worker/pool/pool.go
