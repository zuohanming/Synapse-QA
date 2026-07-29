package repository_test

import (
	"synapseqa/backend/internal/repository"
	"synapseqa/backend/internal/service"
)

var _ service.CandidateCaptureRepository = (*repository.ElementCaptureRepository)(nil)
