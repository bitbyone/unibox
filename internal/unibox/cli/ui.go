package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// unibox asks its questions as a small form at a terminal: a choice among
// what makes sense, with a way to say something else, and each answer
// checked as it is given. With ACCESSIBLE set - for a screen reader, or a
// test - the same questions are plain numbered lines.

// otherChoice is the choice that asks for an answer of one's own.
const otherChoice = "\x00other"

func (o *options) accessible() bool { return os.Getenv("ACCESSIBLE") != "" }

// canAsk says whether a form can be shown: a terminal, or accessible mode.
func (o *options) canAsk() bool { return !o.json && (o.interactive || o.accessible()) }

func (o *options) runField(field huh.Field) error {
	// The theme asks the terminal for its background; asked here, before
	// the form reads keys, the answer is not taken for typing.
	lipgloss.HasDarkBackground()
	f := huh.NewForm(huh.NewGroup(field)).WithShowHelp(true).WithOutput(o.log)
	if o.accessible() {
		// huh reads each answer with a scanner of its own; one byte at a
		// time, none takes what the next question reads.
		f = f.WithAccessible(true).WithInput(byteReader{o.input})
	} else {
		f = f.WithInput(o.input)
	}
	if err := f.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return fmt.Errorf("cancelled; what was answered so far is not written")
		}
		return err
	}
	return nil
}

// askText asks for a line, preset to an answer Enter takes.
func (o *options) askText(title, description, preset string, validate func(string) error) (string, error) {
	value := preset
	field := huh.NewInput().Title(title).Description(description).Value(&value)
	if validate != nil {
		field.Validate(validate)
	}
	if err := o.runField(field); err != nil {
		return "", err
	}
	return value, nil
}

// askChoice offers the options, the preset chosen, and an answer of one's
// own after them, checked by validate.
func (o *options) askChoice(title, description string, options []string, preset string, validate func(string) error) (string, error) {
	// huh scrolls a list to its chosen option, hiding those above it, so
	// the preset comes first.
	if preset != "" {
		options = append([]string{preset}, slices.DeleteFunc(slices.Clone(options), func(v string) bool { return v == preset })...)
	}
	opts := []huh.Option[string]{}
	for _, v := range options {
		opts = append(opts, huh.NewOption(v, v))
	}
	opts = append(opts, huh.NewOption("something else…", otherChoice))
	value := preset
	if value == "" && len(options) > 0 {
		value = options[0]
	}
	if err := o.runField(huh.NewSelect[string]().Title(title).Description(description).Options(opts...).Value(&value).Height(fieldHeight(len(opts), description))); err != nil {
		return "", err
	}
	if value == otherChoice {
		return o.askText(title, description, "", validate)
	}
	if validate != nil {
		if err := validate(value); err != nil {
			return "", err
		}
	}
	return value, nil
}

// pick is an option shown by its label and answered by its value.
type pick struct{ value, label string }

// askPick offers options alone, the preset chosen.
func (o *options) askPick(title, description string, options []pick, preset string) (string, error) {
	opts := []huh.Option[string]{}
	for _, p := range options {
		if p.value == preset {
			opts = append([]huh.Option[string]{huh.NewOption(p.label, p.value)}, opts...)
		} else {
			opts = append(opts, huh.NewOption(p.label, p.value))
		}
	}
	value := preset
	if err := o.runField(huh.NewSelect[string]().Title(title).Description(description).Options(opts...).Value(&value).Height(fieldHeight(len(opts), description))); err != nil {
		return "", err
	}
	return value, nil
}

// askChoices offers options to pick any of, the preset ones picked.
func (o *options) askChoices(title, description string, options, preset []string, validate func([]string) error) ([]string, error) {
	// The picked ones first, as huh scrolls to the first picked.
	picked, rest := []huh.Option[string]{}, []huh.Option[string]{}
	for _, v := range options {
		if slices.Contains(preset, v) {
			picked = append(picked, huh.NewOption(v, v).Selected(true))
		} else {
			rest = append(rest, huh.NewOption(v, v))
		}
	}
	opts := append(picked, rest...)
	value := slices.Clone(preset)
	field := huh.NewMultiSelect[string]().Title(title).Description(description).Options(opts...).Value(&value).Height(fieldHeight(len(opts), description))
	if validate != nil {
		field.Validate(validate)
	}
	if err := o.runField(field); err != nil {
		return nil, err
	}
	// In the order offered, whatever was shown first: the order of agents
	// matters.
	return slices.DeleteFunc(slices.Clone(options), func(v string) bool { return !slices.Contains(value, v) }), nil
}

// askYes asks a yes or no question.
func (o *options) askYes(title string, preset bool) (bool, error) {
	value := preset
	if err := o.runField(huh.NewConfirm().Title(title).Value(&value)); err != nil {
		return false, err
	}
	return value, nil
}

// fieldHeight is a list's height with every option in sight, its title
// and description besides.
func fieldHeight(options int, description string) int {
	return options + 2 + strings.Count(description, "\n") + 1
}

type byteReader struct{ r io.Reader }

func (b byteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return b.r.Read(p)
}
