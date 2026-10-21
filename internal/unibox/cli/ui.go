package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

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
	if preset != "" && !slices.Contains(options, preset) {
		options = append([]string{preset}, options...)
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
	if err := o.runField(huh.NewSelect[string]().Title(title).Description(description).Options(opts...).Value(&value)); err != nil {
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

// askChoices offers options to pick any of, the preset ones picked.
func (o *options) askChoices(title, description string, options, preset []string, validate func([]string) error) ([]string, error) {
	opts := []huh.Option[string]{}
	for _, v := range options {
		opts = append(opts, huh.NewOption(v, v).Selected(slices.Contains(preset, v)))
	}
	value := slices.Clone(preset)
	field := huh.NewMultiSelect[string]().Title(title).Description(description).Options(opts...).Value(&value)
	if validate != nil {
		field.Validate(validate)
	}
	if err := o.runField(field); err != nil {
		return nil, err
	}
	return value, nil
}

// askYes asks a yes or no question.
func (o *options) askYes(title string, preset bool) (bool, error) {
	value := preset
	if err := o.runField(huh.NewConfirm().Title(title).Value(&value)); err != nil {
		return false, err
	}
	return value, nil
}

type byteReader struct{ r io.Reader }

func (b byteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return b.r.Read(p)
}
