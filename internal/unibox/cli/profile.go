package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tobola/unibox/internal/unibox"
	"github.com/tobola/unibox/internal/unibox/api"
	"gopkg.in/yaml.v3"
)

const profilePathHelp = `A path names a field of sandbox.yaml by its keys joined by dots; a key
holding a dot, or a list's item by its index, goes in brackets, as in jq;
empty brackets at the end add an item to a list. A value is YAML: a number,
true, a string, [a, b] or {key: value}.

What is written is checked as unibox reads a profile - the field must exist
and its value be of the right form - and nothing is kept otherwise.
unibox profile validate checks the whole repository.`

// Every field of a profile can be read and written by a command, for a
// program that configures unibox to need no YAML of its own: show gives
// the document, set and unset change any field of it.
func (o *options) profileShowCommand() *cobra.Command {
	return &cobra.Command{Use: "show [profile]", Args: cobra.MaximumNArgs(1), Short: "Print a profile's sandbox.yaml, or with --json the same document as JSON", RunE: func(cmd *cobra.Command, args []string) error {
		name, err := o.profileName(first(args))
		if err != nil {
			return err
		}
		if o.json {
			doc, err := o.manager.ProfileDocument(name)
			if err != nil {
				return err
			}
			return o.write(map[string]any{"api_version": api.Version, "profile": name, "document": json.RawMessage(doc)})
		}
		p, err := o.manager.Profile(name)
		if err != nil {
			return err
		}
		f, err := os.Open(filepath.Join(p.Dir, "sandbox.yaml"))
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(o.output, f)
		return err
	}}
}

func (o *options) profileSetCommand() *cobra.Command {
	var profile string
	c := &cobra.Command{Use: "set path value", Args: cobra.ExactArgs(2), Short: "Set any field of a profile", Long: "Set any field of a profile.\n\n" + profilePathHelp, Example: "  unibox profile set resources.cpus 6\n  unibox profile set agents.claude.args '[--model, opus]'\n  unibox profile set agents.claude.config[settings.json] _data/claude/settings.json\n  unibox profile set network.ports.web \"{port: '3000', default: off}\"\n  unibox profile set env[] BUILD_KIND", RunE: func(cmd *cobra.Command, args []string) error {
		keys, err := unibox.ParseProfilePath(args[0])
		if err != nil {
			return err
		}
		value, err := unibox.ParseProfileValue(args[1])
		if err != nil {
			return err
		}
		name, err := o.profileName(profile)
		if err != nil {
			return err
		}
		if err = o.manager.EditProfile(name, func(doc *yaml.Node) error { return unibox.SetProfilePath(doc, keys, value) }); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("profile %s sets %s", name, args[0]))
	}}
	profileFlag(c, &profile)
	return c
}

func (o *options) profileUnsetCommand() *cobra.Command {
	var profile string
	c := &cobra.Command{Use: "unset path", Args: cobra.ExactArgs(1), Short: "Take any field out of a profile", Long: "Take any field out of a profile, and the mappings it leaves empty.\n\n" + profilePathHelp, Example: "  unibox profile unset image\n  unibox profile unset agents.claude.args", RunE: func(cmd *cobra.Command, args []string) error {
		keys, err := unibox.ParseProfilePath(args[0])
		if err != nil {
			return err
		}
		name, err := o.profileName(profile)
		if err != nil {
			return err
		}
		if err = o.manager.EditProfile(name, func(doc *yaml.Node) error { return unibox.UnsetProfilePath(doc, keys) }); err != nil {
			return err
		}
		return o.success(fmt.Sprintf("profile %s no longer sets %s", name, args[0]))
	}}
	profileFlag(c, &profile)
	return c
}
