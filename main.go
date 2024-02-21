package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"gopkg.in/yaml.v3"
)

var (
	ErrUnsupportedFileExtension = fmt.Errorf("unsupported file extension, only .yaml, .yml and .json are supported")
)

type Survey struct {
	path    string
	answers map[string]interface{}

	Name        string      `yaml:"name" json:"name"`
	Version     string      `yaml:"version" json:"version"`
	Description string      `yaml:"description" json:"description"`
	Output      string      `yaml:"output" json:"output"`
	Questions   []*Question `yaml:"questions" json:"questions"`
	Summary     bool        `yaml:"summary" json:"summary"`
	Confirm     Confirm     `yaml:"confirm" json:"confirm"`
}

type Question struct {
	Key         string         `yaml:"key" json:"key"`
	Type        string         `yaml:"type" json:"type"`
	Title       string         `yaml:"title" json:"title"`
	Description string         `yaml:"description" json:"description"`
	Required    bool           `yaml:"required" json:"required"`
	Placeholder string         `yaml:"placeholder,omitempty" json:"placeholder,omitempty"`
	Default     interface{}    `yaml:"default,omitempty" json:"default,omitempty"`
	Options     []SelectOption `yaml:"options,omitempty" json:"options,omitempty"`

	answer Answer
}

type SelectOption struct {
	Key      string `yaml:"key" json:"key"`
	Value    string `yaml:"value" json:"value"`
	Selected bool   `yaml:"selected" json:"selected"`
}

type Confirm struct {
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
}

type Answer interface {
	Value() interface{}
}

type FieldValueAccessor struct {
	value func() interface{}
}

func (a *FieldValueAccessor) Value() interface{} {
	return a.value()
}

func (s *Survey) Run() error {
	fields := make([]huh.Field, 0)

	header := huh.NewNote().
		Title(fmt.Sprintf("%s, version %s", s.Name, s.Version)).
		Description(fmt.Sprintf("Reading questions from %s, writing answers to %s\n\n%s", s.path, s.Output, s.Description))

	fields = append(fields, header)

	for _, q := range s.Questions {
		if q.Type == "input" {
			fields = append(fields, s.NewInputField(q))
		}

		if q.Type == "text" {
			// TODO
		}

		if q.Type == "select" {
			// TODO
		}

		if q.Type == "multiselect" {
			fields = append(fields, s.NewMultiSelectField(q))
		}

		if q.Type == "confirm" {
			fields = append(fields, s.NewConfirmField(q))
		}
	}

	form := huh.NewForm(
		huh.NewGroup(fields...),
	)

	return form.Run()
}

func (s Survey) NewInputField(q *Question) huh.Field {
	value := ""
	switch q.Default.(type) {
	case string:
		value = q.Default.(string)
	}
	if s.answers != nil {
		if a, ok := s.answers[q.Key].(string); ok {
			value = a
		}
	}
	field := huh.NewInput().
		Title(q.Title).
		Description(q.Description).
		Placeholder(q.Placeholder).
		Value(&value).
		Validate(func(s string) error {
			if q.Required && s == "" {
				return fmt.Errorf("value is required")
			}
			return nil
		})
	q.answer = &FieldValueAccessor{value: field.GetValue}
	return field
}

func (s Survey) NewMultiSelectField(q *Question) huh.Field {
	value := make([]string, 0)
	switch q.Default.(type) {
	case []interface{}:
		df := q.Default.([]interface{})
		for _, v := range df {
			if s, ok := v.(string); ok {
				value = append(value, s)
			}
		}
	}
	options := make([]huh.Option[string], 0)
	for _, o := range q.Options {
		k := o.Key
		if k == "" {
			k = o.Value
		}
		selected := o.Selected

		if s.answers != nil {
			if sel, ok := s.answers[q.Key].([]interface{}); ok {
				for _, v := range sel {
					if v == o.Value {
						selected = true
						break
					}
				}
			}
		}

		options = append(options, huh.NewOption[string](k, o.Value).Selected(selected))
	}

	field := huh.NewMultiSelect[string]().
		Title(q.Title).
		Description(q.Description).
		Options(options...).
		Value(&value).
		Validate(func(t []string) error {
			if q.Required && len(t) <= 0 {
				return fmt.Errorf("at least one items is required")
			}
			return nil
		})

	q.answer = &FieldValueAccessor{value: field.GetValue}
	return field
}

func (s Survey) NewConfirmField(q *Question) huh.Field {
	value := false
	switch q.Default.(type) {
	case bool:
		value = q.Default.(bool)
	}
	if s.answers != nil {
		if a, ok := s.answers[q.Key].(bool); ok {
			value = a
		}
	}
	field := huh.NewConfirm().
		Title(q.Title).
		Description(q.Description).
		Value(&value)

	q.answer = &FieldValueAccessor{value: field.GetValue}
	return field
}

func (s *Survey) Answers() ([]byte, error) {
	o := map[string]interface{}{}

	for _, q := range s.Questions {
		o[q.Key] = q.answer.Value()
	}

	path := s.Output
	if s.Output == "" || s.Output == "-" {
		path = s.path
	}

	// Marshal the file
	switch fileType(path) {
	case "yaml":
		b, err := yaml.Marshal(o)
		if err != nil {
			return []byte{}, err
		}
		return b, nil
	case "json":
		b, err := json.Marshal(o)
		if err != nil {
			return []byte{}, err
		}
		return b, nil
	default:
		return []byte{}, ErrUnsupportedFileExtension
	}
}

func NewSurvey(path string) (Survey, error) {
	s := Survey{
		path: path,
	}

	// Read the file
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}

	// Unmarshal the file
	switch fileType(s.path) {
	case "yaml":
		err = yaml.Unmarshal(b, &s)
		if err != nil {
			return s, err
		}
	case "json":
		err = json.Unmarshal(b, &s)
		if err != nil {
			return s, err
		}
	default:
		return s, ErrUnsupportedFileExtension
	}

	if s.Output != "" && s.Output != "-" {
		// Read the answers
		a, err := readAnswers(s.Output)
		if err != nil {
			return s, err
		}

		s.answers = a
	}

	// Return the survey
	return s, nil
}

func fileType(path string) string {
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		return "yaml"
	} else if strings.HasSuffix(path, ".json") {
		return "json"
	} else {
		return ""
	}
}

func readAnswers(path string) (map[string]interface{}, error) {
	o := map[string]interface{}{}

	// Check if the file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return o, nil
	}

	// Read the file
	b, err := os.ReadFile(path)
	if err != nil {
		return o, err
	}

	// Unmarshal the file
	switch fileType(path) {
	case "yaml":
		err = yaml.Unmarshal(b, &o)
		if err != nil {
			return o, err
		}
	case "json":
		err = json.Unmarshal(b, &o)
		if err != nil {
			return o, err
		}
	default:
		return o, ErrUnsupportedFileExtension
	}

	// Return the answers
	return o, nil
}

func writeSummary(s Survey) {
	w := os.Stdout
	re := lipgloss.NewRenderer(w)
	headerStyle := re.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).Align(lipgloss.Center)
	cellStyle := re.NewStyle().Padding(0, 1).Width(14)
	oddRowStyle := cellStyle.Copy().Foreground(lipgloss.Color("245"))
	evenRowStyle := cellStyle.Copy().Foreground(lipgloss.Color("242"))
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	qColWidth := 20
	aColWidht := 20
	kColWidth := 20
	rows := [][]string{}

	for _, q := range s.Questions {
		answer := fmt.Sprintf("%v", q.answer.Value())
		rows = append(rows, []string{q.Title, answer, q.Key})
		if len(q.Title) > qColWidth {
			qColWidth = len(q.Title) + 4
		}
		if len(answer) > aColWidht {
			aColWidht = len(answer) + 4
		}
		if len(q.Key) > kColWidth {
			kColWidth = len(q.Key) + 4
		}
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(borderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			var style lipgloss.Style
			switch {
			case row == 0:
				return headerStyle
			case row%2 == 0:
				style = evenRowStyle
			default:
				style = oddRowStyle
			}

			switch col {
			case 0:
				style = style.Copy().Width(qColWidth)
			case 1:
				style = style.Copy().Width(aColWidht)
			case 2:
				style = style.Copy().Width(kColWidth)
			}
			return style
		}).
		Headers("Question", "Answer", "Key").
		Rows(rows...)

	fmt.Fprintln(w, t)
	fmt.Fprintln(w)
}

func main() {
	path := "survey.yaml"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	s, err := NewSurvey(path)
	if err != nil {
		log.Fatal(err)
	}

	if err := s.Run(); err != nil {
		log.Fatal(err)
	}

	if s.Summary {
		writeSummary(s)
	}

	ok := true
	if len(s.Confirm.Title) > 0 {
		confirm := huh.NewConfirm().
			Title(s.Confirm.Title).
			Description(s.Confirm.Description).
			Value(&ok)

		if err := confirm.Run(); err != nil {
			log.Fatal(err)
		}

		if !ok {
			return
		}
	}

	a, err := s.Answers()
	if err != nil {
		log.Fatal(err)
	}

	if s.Output == "" || s.Output == "-" {
		os.Stdout.Write(a)
		return
	} else {
		err = os.WriteFile(s.Output, []byte(a), 0644)
		if err != nil {
			log.Fatal(err)
		}
		return
	}
}
