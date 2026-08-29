package discovery

import (
	"context"
	"errors"
	"fmt"

	"vps-dashboard-api/internal/models"
)

// AdoptResult summarises one candidate-to-project insertion attempt.
type AdoptResult struct {
	Name   string `json:"name"`
	ID     string `json:"id,omitempty"`
	Status string `json:"status"` // "created", "skipped", "error"
	Error  string `json:"error,omitempty"`
}

// AdoptAllReport is the aggregate outcome of AdoptAll.
type AdoptAllReport struct {
	Adopted []AdoptResult
	Skipped []AdoptResult
	Errors  []AdoptResult
}

// Total returns the number of candidates processed.
func (r AdoptAllReport) Total() int {
	return len(r.Adopted) + len(r.Skipped) + len(r.Errors)
}

// AdoptAll captures a snapshot and inserts every non-adopted candidate
// into the projects registry. Candidates that are already adopted, that
// fail validation, or that collide on the unique name are reported but
// not fatal: AdoptAll only returns an error if the snapshot itself
// could not be produced.
//
// This is the entry point used by the auto-seed startup hook so the
// dashboard ships with a populated projects table on first boot.
func (s *Service) AdoptAll(ctx context.Context, repo *models.ProjectRepo) (AdoptAllReport, error) {
	if s == nil {
		return AdoptAllReport{}, errors.New("discovery: nil service")
	}
	if repo == nil {
		return AdoptAllReport{}, errors.New("discovery: nil project repo")
	}

	snap, err := s.Capture(ctx, repo)
	if err != nil {
		return AdoptAllReport{}, fmt.Errorf("discovery: capture: %w", err)
	}

	report := AdoptAllReport{
		Adopted: []AdoptResult{},
		Skipped: []AdoptResult{},
		Errors:  []AdoptResult{},
	}

	for _, cand := range snap.Candidates {
		name := cand.SuggestedName

		if cand.AlreadyAdopted {
			report.Skipped = append(report.Skipped, AdoptResult{
				Name:   name,
				ID:     cand.AdoptedAs,
				Status: "skipped",
				Error:  "already_adopted",
			})
			continue
		}

		p := candidateToProject(cand)
		if err := p.Validate(); err != nil {
			report.Errors = append(report.Errors, AdoptResult{
				Name:   name,
				Status: "error",
				Error:  "invalid: " + err.Error(),
			})
			continue
		}

		created, err := repo.Create(ctx, p)
		if err != nil {
			if errors.Is(err, models.ErrDuplicateName) {
				report.Skipped = append(report.Skipped, AdoptResult{
					Name:   p.Name,
					Status: "skipped",
					Error:  "duplicate_name",
				})
				continue
			}
			report.Errors = append(report.Errors, AdoptResult{
				Name:   p.Name,
				Status: "error",
				Error:  err.Error(),
			})
			continue
		}

		report.Adopted = append(report.Adopted, AdoptResult{
			Name:   created.Name,
			ID:     created.ID,
			Status: "created",
		})
	}

	return report, nil
}

// candidateToProject mirrors the field mapping used by the HTTP adopt
// handler so auto-seed produces rows shaped identically to manually
// adopted ones.
func candidateToProject(c Candidate) models.Project {
	return models.Project{
		Name:          c.SuggestedName,
		Domain:        c.Domain,
		Port:          c.Port,
		ContainerName: c.ContainerName,
		PM2Name:       c.PM2Name,
		TunnelService: c.TunnelService,
		HealthURL:     c.HealthURL,
		Enabled:       true,
		Tags:          []string{},
	}
}
