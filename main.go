package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"gopkg.in/yaml.v3"
)

const (
	DefaultTheme = "charm"
)

var (
	ErrUnsupportedFileExtension = fmt.Errorf("unsupported file extension, only .yaml, .yml and .json are supported")
)

type Survey struct {
	path    string
	answers map[string]interface{}

	Name        string  `yaml:"name" json:"name"`
	Version     string  `yaml:"version" json:"version"`
	Description string  `yaml:"description" json:"description"`
	Theme       string  `yaml:"theme" json:"theme"`
	Accessible  bool    `yaml:"accessible" json:"accessible"`
	Output      string  `yaml:"output" json:"output"`
	Forms       []*Form `yaml:"forms" json:"forms"`
	Summary     bool    `yaml:"summary" json:"summary"`
	Confirm     Confirm `yaml:"confirm" json:"confirm"`
}

type Form struct {
	Groups []*Group `yaml:"groups" json:"groups"`
}

func (f *Form) ValueFields() []*Field {
	fields := make([]*Field, 0)
	for _, g := range f.Groups {
		fields = append(fields, g.ValueFields()...)
	}
	return fields
}

type Group struct {
	Title       string   `yaml:"title" json:"title"`
	Description string   `yaml:"description" json:"description"`
	Fields      []*Field `yaml:"fields" json:"fields"`
}

func (g *Group) ValueFields() []*Field {
	fields := make([]*Field, 0)
	for _, f := range g.Fields {
		switch f.Type {
		case "note":
			continue
		default:
			fields = append(fields, f)
		}
	}
	return fields
}

type Field struct {
	ref         huh.Field
	Key         string         `yaml:"key" json:"key"`
	Type        string         `yaml:"type" json:"type"`
	Title       string         `yaml:"title" json:"title"`
	Description string         `yaml:"description" json:"description"`
	Required    bool           `yaml:"required" json:"required"`
	Placeholder string         `yaml:"placeholder,omitempty" json:"placeholder,omitempty"`
	Default     interface{}    `yaml:"default,omitempty" json:"default,omitempty"`
	Options     []SelectOption `yaml:"options,omitempty" json:"options,omitempty"`
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

func (s *Survey) Run() error {
	theme := getTheme(s.Theme)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(strings.TrimSpace(fmt.Sprintf("%s, version %s", s.Name, s.Version))).
				Description(strings.TrimSpace(fmt.Sprintf("Reading questions from %s, writing answers to %s\n\n%s", s.path, s.Output, s.Description))),
		),
	).WithTheme(&theme).WithAccessible(s.Accessible)

	if err := form.Run(); err != nil {
		return err
	}

	for _, f := range s.Forms {
		var groups []*huh.Group

		for _, g := range f.Groups {
			fields := make([]huh.Field, 0)

			for _, field := range g.Fields {
				switch field.Type {
				case "note":
					fields = append(fields, s.NewNoteField(field))
				case "input":
					fields = append(fields, s.NewInputField(field))
				case "text":
					fields = append(fields, s.NewTextField(field))
				case "select":
					fields = append(fields, s.NewSelectField(field))
				case "multiselect":
					fields = append(fields, s.NewMultiSelectField(field))
				case "confirm":
					fields = append(fields, s.NewConfirmField(field))
				default:
					return fmt.Errorf("unsupported field type: %s", field.Type)
				}
			}

			if len(fields) > 0 {
				groups = append(groups, huh.NewGroup(fields...).Title(g.Title).Description(g.Description))
			}
		}

		if len(groups) > 0 {
			// Collect answers for the form
			form := huh.NewForm(groups...).WithTheme(&theme).WithAccessible(s.Accessible)
			if err := form.Run(); err != nil {
				return err
			}

			// Store the answers
			for _, field := range f.ValueFields() {
				s.answers[field.Key] = field.ref.GetValue()
			}
		}
	}

	return nil
}

func (s Survey) NewNoteField(f *Field) huh.Field {
	field := huh.NewNote().
		Title(strings.TrimSpace(f.Title)).
		Description(strings.TrimSpace(f.Description))
	f.ref = field
	return field
}

func (s Survey) NewInputField(f *Field) huh.Field {
	value := ""
	switch f.Default.(type) {
	case string:
		value = f.Default.(string)
		v, err := s.ParseTemplate(value, f.Key)
		if err != nil {
			panic(err)
		}
		value = v
	}
	if s.answers != nil {
		if a, ok := s.answers[f.Key].(string); ok {
			value = a
		}
	}
	// TODO: Add support for password
	field := huh.NewInput().
		Title(strings.TrimSpace(f.Title)).
		Description(strings.TrimSpace(f.Description)).
		Placeholder(f.Placeholder).
		Value(&value).
		Validate(func(s string) error {
			if f.Required && s == "" {
				return fmt.Errorf("value is required")
			}
			return nil
		})
	f.ref = field
	return field
}

func (s Survey) NewTextField(f *Field) huh.Field {
	value := ""
	switch f.Default.(type) {
	case string:
		value = f.Default.(string)
		v, err := s.ParseTemplate(value, f.Key)
		if err != nil {
			panic(err)
		}
		value = v
	}
	if s.answers != nil {
		if a, ok := s.answers[f.Key].(string); ok {
			value = a
		}
	}
	field := huh.NewText().
		Title(strings.TrimSpace(f.Title)).
		Description(strings.TrimSpace(f.Description)).
		Placeholder(f.Placeholder).
		Value(&value).
		Validate(func(s string) error {
			if f.Required && s == "" {
				return fmt.Errorf("value is required")
			}
			return nil
		})
	f.ref = field
	return field
}

func (s Survey) NewSelectField(f *Field) huh.Field {
	value := ""
	switch f.Default.(type) {
	case string:
		df := f.Default.(string)
		value = df
	}
	options := make([]huh.Option[string], 0)
	for _, o := range f.Options {
		k := o.Key
		if k == "" {
			k = o.Value
		}
		selected := o.Selected

		if s.answers != nil {
			if sel, ok := s.answers[f.Key].([]interface{}); ok {
				value = value[:0]
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

	field := huh.NewSelect[string]().
		Title(strings.TrimSpace(f.Title)).
		Description(strings.TrimSpace(f.Description)).
		Options(options...).
		Value(&value).
		Validate(func(s string) error {
			if f.Required && len(s) <= 0 {
				return fmt.Errorf("a single item is required")
			}
			return nil
		})

	f.ref = field
	return field
}

func (s Survey) NewMultiSelectField(f *Field) huh.Field {
	value := make([]string, 0)
	switch f.Default.(type) {
	case []interface{}:
		df := f.Default.([]interface{})
		for _, v := range df {
			if s, ok := v.(string); ok {
				value = append(value, s)
			}
		}
	}
	options := make([]huh.Option[string], 0)
	for _, o := range f.Options {
		k := o.Key
		if k == "" {
			k = o.Value
		}
		selected := o.Selected

		if s.answers != nil {
			if sel, ok := s.answers[f.Key].([]interface{}); ok {
				value = value[:0]
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
		Title(strings.TrimSpace(f.Title)).
		Description(strings.TrimSpace(f.Description)).
		Options(options...).
		Value(&value).
		Validate(func(t []string) error {
			if f.Required && len(t) <= 0 {
				return fmt.Errorf("at least one items is required")
			}
			return nil
		})

	f.ref = field
	return field
}

func (s Survey) NewConfirmField(f *Field) huh.Field {
	value := false
	switch f.Default.(type) {
	case bool:
		value = f.Default.(bool)
	}
	if s.answers != nil {
		if a, ok := s.answers[f.Key].(bool); ok {
			value = a
		}
	}
	field := huh.NewConfirm().
		Title(f.Title).
		Description(strings.TrimSpace(f.Description)).
		Value(&value)

	f.ref = field
	return field
}

func (s *Survey) ParseTemplate(value string, key string) (string, error) {
	if len(value) < 1 {
		return value, nil
	}

	tmpl, err := template.New(key).Parse(value)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, s.answers)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (s *Survey) Answers() ([]byte, error) {
	o := map[string]interface{}{}

	for _, f := range s.Forms {
		for _, g := range f.Groups {
			for _, field := range g.Fields {
				o[field.Key] = field.ref.GetValue()
			}
		}
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
		path:    path,
		answers: map[string]interface{}{},
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

func getTheme(name string) huh.Theme {
	if name == "" {
		name = DefaultTheme
	}
	switch name {
	case "base":
		return *huh.ThemeBase()
	case "base16":
		return *huh.ThemeBase16()
	case "charm":
		return *huh.ThemeCharm()
	case "catppuccin":
		return *huh.ThemeCatppuccin()
	case "dracula":
		return *huh.ThemeDracula()
	default:
		panic(fmt.Errorf("unsupported theme: %s", name))
	}
}

func writeGroupSummary(g *Group, theme *huh.Theme) {
	w := os.Stdout
	re := lipgloss.NewRenderer(w)
	titleStyle := re.NewStyle().Inherit(theme.Focused.Title)
	descriptionStyle := re.NewStyle().Inherit(theme.Focused.Description)
	baseStyle := re.NewStyle()

	headerStyle := titleStyle.Copy().Bold(true).Align(lipgloss.Center)
	cellStyle := baseStyle.Copy().Padding(0, 1).Width(14)
	oddRowStyle := cellStyle.Copy().Foreground(lipgloss.Color("245"))
	evenRowStyle := cellStyle.Copy().Foreground(lipgloss.Color("242"))
	borderStyle := descriptionStyle.Copy()
	qColWidth := 20
	aColWidht := 20
	kColWidth := 20
	rows := [][]string{}

	if g.Title != "" {
		text := titleStyle.Render(g.Title)
		if g.Description != "" {
			text = fmt.Sprintf("%s - %s", titleStyle.Render(g.Title), descriptionStyle.Render(g.Description))
		}
		fmt.Fprintln(w, text)
	}

	for _, field := range g.ValueFields() {
		answer := fmt.Sprintf("%v", field.ref.GetValue())
		rows = append(rows, []string{field.Title, answer, field.Key})
		if len(field.Title) > qColWidth {
			qColWidth = len(field.Title) + 4
		}
		if len(answer) > aColWidht {
			aColWidht = len(answer) + 4
		}
		if len(field.Key) > kColWidth {
			kColWidth = len(field.Key) + 4
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

	surveyTheme := getTheme(s.Theme)
	if s.Summary {
		for _, f := range s.Forms {
			for _, g := range f.Groups {
				writeGroupSummary(g, &surveyTheme)
			}
		}
	}

	ok := true
	if len(s.Confirm.Title) > 0 {
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(strings.TrimSpace(s.Confirm.Title)).
					Description(strings.TrimSpace(s.Confirm.Description)).
					Value(&ok),
			),
		).WithTheme(&surveyTheme).WithAccessible(s.Accessible)

		if err := form.Run(); err != nil {
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
