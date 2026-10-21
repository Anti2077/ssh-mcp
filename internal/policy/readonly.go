package policy

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// SSHReadOnly positively recognizes a deliberately small diagnostic language.
// Unknown commands, options with side effects, shell expansion and redirects
// require user authorization. This classifier is not an OS security sandbox.
func SSHReadOnly(command string, asRoot bool, environment map[string]string) bool {
	if asRoot || len(environment) != 0 {
		return false
	}
	file, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Parse(strings.NewReader(command), "")
	if err != nil {
		return false
	}
	ok, calls := true, 0
	syntax.Walk(file, func(node syntax.Node) bool {
		if !ok {
			return false
		}
		switch n := node.(type) {
		case nil, *syntax.File, *syntax.Word, *syntax.Lit, *syntax.SglQuoted, *syntax.DblQuoted:
		case *syntax.Stmt:
			if n.Background || n.Coprocess || len(n.Redirs) != 0 {
				ok = false
			}
		case *syntax.BinaryCmd:
			if n.Op != syntax.Pipe && n.Op != syntax.AndStmt && n.Op != syntax.OrStmt {
				ok = false
			}
		case *syntax.CallExpr:
			calls++
			if len(n.Assigns) != 0 || !literalShellWordsStatic(n.Args) || !readOnlyCommand(literalShellWords(n.Args)) {
				ok = false
			}
		default:
			ok = false
		}
		return ok
	})
	return ok && calls > 0
}

func readOnlyCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	// Do not accept paths or wrappers: /tmp/cat and env PATH=... cat are not
	// known diagnostic commands. Remote account/PATH integrity remains required.
	switch args[0] {
	case "cat", "head", "tail", "grep", "egrep", "fgrep", "ls", "stat", "wc", "pwd", "whoami", "id", "uname", "uptime", "free", "df", "du", "ps", "hostname", "nproc", "lscpu", "lsblk", "readlink", "realpath", "sha256sum", "md5sum":
		// These commands have no ordinary file-writing flags, with the
		// exceptions below. Checksum verification and hostname mutation differ.
		if args[0] == "hostname" {
			return len(args) == 1
		}
		for _, arg := range args[1:] {
			if (args[0] == "sha256sum" || args[0] == "md5sum") && strings.HasPrefix(arg, "-") && arg != "--" && arg != "-b" && arg != "-t" {
				return false
			}
			if strings.ContainsAny(arg, "*?[") {
				return false
			}
		}
		return true
	case "journalctl":
		for i := 1; i < len(args); i++ {
			arg := args[i]
			switch arg {
			case "--no-pager", "--utc", "--reverse", "-r", "--boot", "-b", "--quiet", "-q", "--follow", "-f", "--system", "--user":
			case "-u", "--unit", "-n", "--lines", "-p", "--priority", "--since", "--until", "-o", "--output":
				i++
				if i >= len(args) || strings.HasPrefix(args[i], "-") {
					return false
				}
			default:
				// Reject unfamiliar switches instead of maintaining a growing
				// blacklist of journal modification options.
				return false
			}
		}
		// Always suppress pager execution, including remote user configuration.
		return containsArgument(args, "--no-pager")
	case "systemctl":
		if len(args) < 3 || !containsArgument(args, "--no-pager") {
			return false
		}
		switch args[1] {
		case "status", "show", "is-active", "is-enabled", "list-units", "list-unit-files":
			return true
		}
	}
	return false
}

func containsArgument(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}
