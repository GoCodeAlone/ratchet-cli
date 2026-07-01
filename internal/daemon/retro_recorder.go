package daemon

import (
	"log"
	"path/filepath"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	"github.com/GoCodeAlone/ratchet-cli/internal/retro"
	"github.com/GoCodeAlone/workflow/secrets"
)

func newRetroRecorder(cfg config.RetroConfig, dataDir string, redactor *secrets.Redactor) *retro.Recorder {
	if !cfg.Enabled {
		return nil
	}
	path := filepath.Join(dataDir, "retro", "evidence.jsonl")
	return retro.NewRecorder(retro.NewEvidenceStore(path, redactor))
}

func (s *Service) recordRetroEvidence(event retro.Event) {
	if s == nil || s.retroRecorder == nil {
		return
	}
	if err := s.retroRecorder.Record(event); err != nil {
		log.Printf("retro evidence: %v", err)
	}
}
