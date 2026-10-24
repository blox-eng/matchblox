package sample

import "strings"

// engine is what is known of how one agent runs.
type engine struct {
	Name      string              // as in config agents
	Batch     func([]string) bool // argv runs it with no person at it
	Server    func([]string) bool // argv is a daemon or server: never a session
	ParentEnv string              // the variable it exports to its shells naming its own pid
}

var engines = map[string]engine{
	"claude": {Name: "claude", Batch: printRun, ParentEnv: "CLAUDE_PID"},
	"codex": {
		Name:  "codex",
		Batch: func(argv []string) bool { return codexBatch(codexArgv(argv)) },
		Server: func(argv []string) bool {
			w := codexWords(codexArgv(argv))
			return len(w) > 0 && (w[0] == "app-server" || w[0] == "mcp-server")
		},
	},
	"opencode": {Name: "opencode", Batch: opencodeRun},
}

// engineOf is name's entry, or a zero entry with only Name: linked by the
// process tree only, no batch or server rule.
func engineOf(name string) engine {
	if e, ok := engines[name]; ok {
		return e
	}
	return engine{Name: name}
}

func printRun(argv []string) bool {
	return len(argv) > 0 && cursorPrint(argv)
}

// opencodeRun: the first word after the program, or after its script when
// an interpreter runs it, is `run`.
func opencodeRun(argv []string) bool {
	argv = codexArgv(argv)
	for _, a := range argv[min(1, len(argv)):] {
		if !strings.HasPrefix(a, "-") {
			return a == "run"
		}
	}
	return false
}
