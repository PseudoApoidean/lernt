package main

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// The highlight colour's three modes, in the order the setting's wheel has them, and
// what each is called when it is saved.
const (
	themeMode = iota
	customMode
	wheelMode
)

var (
	colourModeNames  = []string{"Terminal theme", "Custom", "Colour wheel"}
	colourModeValues = []string{"theme", "custom", "wheel"}
)

// The keys the colour is saved under. The custom list is one key, as JSON.
const (
	keyColourMode   = "colour_mode"
	keyColourTheme  = "colour_theme"
	keyColourCustom = "colour_custom"
	keyColourChoice = "colour_custom_choice"
	keyColourWheel  = "colour_wheel"
)

const maxColourName = 24

// A customColour is a colour the user added: what they called it, and the colour in
// the one form the settings keep (see normalColour).
type customColour struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// themeWheel is the terminal's 16 colours, on the default.
func themeWheel() wheel {
	var w wheel
	for i, c := range themeColours {
		w.labels, w.values = append(w.labels, c.name), append(w.values, i)
		if c.name == defaultColour {
			w.at = i
		}
	}
	return w
}

// highlight is the colour the settings say to draw with, in the form applyTheme takes:
// the terminal colour's name, the custom colour's value, or the one picked on the
// wheel. With no custom colour added yet, the default.
func (p prefs) highlight() string {
	switch p.colourMode {
	case customMode:
		if len(p.custom) > 0 {
			return p.custom[p.customAt].Value
		}
	case wheelMode:
		return p.picked
	case themeMode:
		return p.theme.label()
	}
	return defaultColour
}

// addCustom adds a colour to the end of the list, and makes it the one chosen. It is
// refused with a reason if the name is empty, too long, or one already there (however
// it is capitalised), or the colour is not one that can be read.
func (p *prefs) addCustom(name, value string) error {
	// The colour is checked first, as it is the first box of the form.
	normal, ok := normalColour(value)
	if !ok {
		return errors.New("That is not a colour: try #ff8800, 208 or 255,136,0.")
	}
	name = strings.Join(strings.Fields(name), " ")
	switch {
	case name == "":
		return errors.New("A colour needs a name.")
	case len([]rune(name)) > maxColourName:
		return errors.New("Keep the name to 24 characters.")
	case slices.ContainsFunc(p.custom, func(c customColour) bool { return strings.EqualFold(c.Name, name) }):
		return errors.New("There is already a colour called " + name + ".")
	}
	p.custom = append(p.custom, customColour{name, normal})
	p.customAt = len(p.custom) - 1
	return nil
}

// deleteCustom removes the colour chosen. The one before it, or the next, is chosen.
// A list is only added to and taken from, never edited in place: a colour to change is
// removed and added again.
func (p *prefs) deleteCustom() {
	if len(p.custom) == 0 {
		return
	}
	p.custom = slices.Delete(p.custom, p.customAt, p.customAt+1)
	p.customAt = max(min(p.customAt, len(p.custom)-1), 0)
}

// encodeCustom and decodeCustom are the list as the one string the settings keep. A
// list that cannot be read, and any entry in it that cannot, is dropped, and so is a
// name that comes twice.
func encodeCustom(list []customColour) string {
	if len(list) == 0 {
		return "[]"
	}
	out, _ := json.Marshal(list)
	return string(out)
}

func decodeCustom(s string) []customColour {
	var raw []customColour
	if json.Unmarshal([]byte(s), &raw) != nil {
		return nil
	}
	var out []customColour
	for _, c := range raw {
		name := strings.Join(strings.Fields(c.Name), " ")
		value, ok := normalColour(c.Value)
		if name == "" || len([]rune(name)) > maxColourName || !ok ||
			slices.ContainsFunc(out, func(o customColour) bool { return strings.EqualFold(o.Name, name) }) {
			continue
		}
		out = append(out, customColour{name, value})
	}
	return out
}

// restoreColours reads the colour keys of the saved settings, passing over a value it
// cannot read.
func (p *prefs) restoreColours(key, value string) {
	value = strings.TrimSpace(value)
	switch key {
	case keyColourMode:
		if i := slices.Index(colourModeValues, value); i >= 0 {
			p.colourMode = i
		}
	case keyColourTheme:
		if i := slices.Index(p.theme.labels, strings.ToLower(value)); i >= 0 {
			p.theme.at = i
		}
	case keyColourCustom:
		p.custom = decodeCustom(value)
		p.customAt = max(min(p.customAt, len(p.custom)-1), 0)
	case keyColourChoice:
		if i := slices.IndexFunc(p.custom, func(c customColour) bool { return strings.EqualFold(c.Name, value) }); i >= 0 {
			p.customAt = i
		}
	case keyColourWheel:
		if normal, ok := normalColour(value); ok && strings.HasPrefix(normal, "#") {
			p.picked = normal
		}
	}
}

// colourSaved is the colour settings as key and value.
func (p prefs) colourSaved() (out []struct{ Key, Value string }) {
	choice := ""
	if len(p.custom) > 0 {
		choice = p.custom[p.customAt].Name
	}
	return []struct{ Key, Value string }{
		{keyColourMode, colourModeValues[p.colourMode]},
		{keyColourTheme, p.theme.label()},
		{keyColourCustom, encodeCustom(p.custom)},
		{keyColourChoice, choice},
		{keyColourWheel, p.picked},
	}
}
