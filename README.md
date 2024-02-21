# dhuh?

Declarative survey builder, using huh? and lipgloss.

## Goal

- Make it easy to collect user data without the need for complicated scripts
- Data validation
- Easy access to and formatting of collected data

## Use cases

- Collecting data for scaffolding
- Collecting feedback without leaving the terminal

## State

Currently it's more a proof of concept. Lot's of stuff missing

## Roadmap

No real plan has formulated yet, but there are some key things missing

- Support for missing field types provided by huh
- Command for reading (and formatting) answers
- Validation

More ideas

- More output options (call webhook, send email, ...)
- Support for form groups (with custom actions running after each group)

## Give it a spin

```bash
go mod download
go run . survey.yaml
```