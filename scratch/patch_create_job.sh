sed -i '/We DO NOT reject if they aren.t completed yet/a \
		for _, dep := range deps {\
			if dep.Status != jobs.StatusCompleted {\
				allMet = false\
				break\
			}\
		}' internal/service/job_service.go

sed -i '/if len(dependencies) > 0 {/i \
	allMet := true' internal/service/job_service.go

sed -i 's/if job.Status != jobs.StatusRecurring {/if job.Status != jobs.StatusRecurring \&\& allMet {/g' internal/service/job_service.go
