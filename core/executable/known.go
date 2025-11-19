package executable

import (
	"fmt"
)

type IDE string

const (
	PyCharm   IDE = "pycharm"
	IntelliJ  IDE = "intellij"
	GoLand    IDE = "goland"
	WebStorm  IDE = "webstorm"
	PhpStorm  IDE = "phpstorm"
	RubyMine  IDE = "rubymine"
	CLion     IDE = "clion"
	Rider     IDE = "rider"
	DataGrip  IDE = "datagrip"
	AppCode   IDE = "appcode"
	Windsurf  IDE = "windsurf"
	Cursor    IDE = "cursor"
	CursorCLI IDE = "cursor-cli"
	Claude    IDE = "claude"
	Codex     IDE = "codex"
)

func GetJetbrainsIDEs() []IDE {
	return []IDE{
		PyCharm,
		IntelliJ,
		GoLand,
		WebStorm,
		PhpStorm,
		RubyMine,
		CLion,
		Rider,
		DataGrip,
		AppCode,
	}
}

func getKnown() []IDE {
	return append(GetJetbrainsIDEs(), CursorCLI, Cursor, Windsurf, Claude, Codex)
}

func asIDE(ide string) (IDE, error) {
	for _, i := range getKnown() {
		if ide == string(i) {
			return i, nil
		}
	}
	return "", fmt.Errorf("unknown IDE: %s", ide)
}
