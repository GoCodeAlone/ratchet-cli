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
	for i := range msg.ToolCalls {
		msg.ToolCalls[i].Arguments = r.redactArguments(msg.ToolCalls[i].Arguments)
	}
}

func (r *engineSecretRedactor) redactArguments(args map[string]any) map[string]any {
	for k, v := range args {
		args[k] = r.redactValue(v)
	}
	return args
}

func (r *engineSecretRedactor) redactValue(v any) any {
	switch typed := v.(type) {
	case string:
		return r.Redact(typed)
	case map[string]any:
		return r.redactArguments(typed)
	case []any:
		for i, item := range typed {
			typed[i] = r.redactValue(item)
		}
		return typed
	case []string:
		for i, item := range typed {
			typed[i] = r.Redact(item)
		}
		return typed
	case map[string]string:
		for k, item := range typed {
			typed[k] = r.Redact(item)
		}
		return typed
	default:
		return v
	}
}
