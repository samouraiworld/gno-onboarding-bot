package templates

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	"gopkg.in/yaml.v3"
)

type rawTemplates struct {
	Welcome            string `yaml:"welcome"`
	Acknowledge        string `yaml:"acknowledge"`
	RequestMissingInfo string `yaml:"request_missing_info"`
	Approve            string `yaml:"approve"`
	Decline            string `yaml:"decline"`
	EscalateToCall     string `yaml:"escalate_to_call"`
	Activated          string `yaml:"activated"`
	RoleRemoved        string `yaml:"role_removed"`
}

type Templates struct {
	welcome            *template.Template
	acknowledge        *template.Template
	requestMissingInfo *template.Template
	approve            *template.Template
	decline            *template.Template
	escalateToCall     *template.Template
	activated          *template.Template
	roleRemoved        *template.Template
}

func Load(path string) (*Templates, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read templates file: %w", err)
	}
	var raw rawTemplates
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse templates file: %w", err)
	}

	t := &Templates{}
	entries := []struct {
		name string
		text string
		dest **template.Template
	}{
		{"welcome", raw.Welcome, &t.welcome},
		{"acknowledge", raw.Acknowledge, &t.acknowledge},
		{"request_missing_info", raw.RequestMissingInfo, &t.requestMissingInfo},
		{"approve", raw.Approve, &t.approve},
		{"decline", raw.Decline, &t.decline},
		{"escalate_to_call", raw.EscalateToCall, &t.escalateToCall},
		{"activated", raw.Activated, &t.activated},
		{"role_removed", raw.RoleRemoved, &t.roleRemoved},
	}
	for _, e := range entries {
		parsed, err := template.New(e.name).Parse(e.text)
		if err != nil {
			return nil, fmt.Errorf("parse template %q: %w", e.name, err)
		}
		*e.dest = parsed
	}
	return t, nil
}

func render(t *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render template %q: %w", t.Name(), err)
	}
	return buf.String(), nil
}

func (t *Templates) Welcome() (string, error) {
	return render(t.welcome, nil)
}

func (t *Templates) Acknowledge() (string, error) {
	return render(t.acknowledge, nil)
}

func (t *Templates) RequestMissingInfo(items []string) (string, error) {
	return render(t.requestMissingInfo, struct{ Items []string }{items})
}

func (t *Templates) Approve() (string, error) {
	return render(t.approve, nil)
}

func (t *Templates) Decline(criteria []string) (string, error) {
	return render(t.decline, struct{ Criteria []string }{criteria})
}

func (t *Templates) EscalateToCall(topic, options, scope string) (string, error) {
	return render(t.escalateToCall, struct{ Topic, Options, Scope string }{topic, options, scope})
}

func (t *Templates) Activated() (string, error) {
	return render(t.activated, nil)
}

func (t *Templates) RoleRemoved(name, announcementLink string) (string, error) {
	return render(t.roleRemoved, struct{ Name, AnnouncementLink string }{name, announcementLink})
}
