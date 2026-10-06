package internal

import "fmt"

// Completion prints a shell completion script for the given shell.
// Kept deliberately small: subcommands and flags, no dynamic candidates.
func Completion(shell string) (string, error) {
	switch shell {
	case "bash":
		return `_docsync() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  if [ "$COMP_CWORD" -eq 1 ]; then
    COMPREPLY=($(compgen -W "check report fix version completion" -- "$cur"))
    return 0
  fi
  case "$prev" in
    --config|--baseline|--baseline-out) COMPREPLY=($(compgen -f -- "$cur")); return 0 ;;
    --format) COMPREPLY=($(compgen -W "text json sarif" -- "$cur")); return 0 ;;
    completion) COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur")); return 0 ;;
  esac
  COMPREPLY=($(compgen -W "--json --format --config --baseline --baseline-out --dry-run" -- "$cur"))
}
complete -F _docsync docsync
`, nil
	case "zsh":
		return `#compdef docsync
_docsync() {
  local -a cmds=(check report fix version completion)
  if (( CURRENT == 2 )); then
    _describe 'command' cmds
    return
  fi
  _arguments \
    '--json[structured output]' \
    '--format[output format]:format:(text json sarif)' \
    '--config[config file]:file:_files' \
    '--baseline[baseline file]:file:_files' \
    '--baseline-out[write baseline]:file:_files' \
    '--dry-run[print only]'
}
_docsync "$@"
`, nil
	case "fish":
		return `complete -c docsync -n '__fish_use_subcommand' -a 'check report fix version completion'
complete -c docsync -l json -d 'structured output'
complete -c docsync -l format -a 'text json sarif' -d 'output format'
complete -c docsync -l config -r -F -d 'config file'
complete -c docsync -l baseline -r -F -d 'baseline file'
complete -c docsync -l baseline-out -r -F -d 'write baseline'
complete -c docsync -l dry-run -d 'print only'
`, nil
	}
	return "", fmt.Errorf("unknown shell %q — use bash, zsh, or fish", shell)
}
