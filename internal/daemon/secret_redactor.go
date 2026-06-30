package daemon

import (
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
	"github.com/GoCodeAlone/workflow/secrets"
)

type engineSecretRedactor struct {
	redactor *secrets.Redactor
}

func newEngineSecretRedactor(redactor *secrets.Redactor) *engineSecretRedactor {
	return &engineSecretRedactor{redactor: redactor}
}

func (r *engineSecretRedactor) Redact(text string) string {
	if r == nil || r.redactor == nil {
		return text
	}
	return r.redactor.Redact(text)
}

func (r *engineSecretRedactor) CheckAndRedact(msg *provider.Message) {
	if msg == nil {
		return
	}
	msg.Content = r.Redact(msg.Content)
}
